package otflevel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
	"github.com/nsip/otf-level/internal/util"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
)

type OtfLevelService struct {
	// embedded web server to handle alignment requests
	e *echo.Echo
	// the unique name of this service when running multiple instances
	serviceName string
	// the unique id of this service when running multiple instances
	serviceID string
	// the host address this service instance is running on
	serviceHost string
	// the port that this service instance is running on
	servicePort int
	// the host address of the nias3 server used for data lookups
	niasHost string
	// the port the nias3 server is running on
	niasPort int
	// the jwt used to acess the nias service
	niasToken string
}

//
// Query paramters sent to the
// web service.
// Params can be provided as json payload, via form components
// or as query params
//
type LevelRequest struct {
	//
	// method to be used for levelling one of...
	// prescribed: results in lookup/passthrough of NLP reference
	// mapped: maps from input token through known scales such as NAPLAN to find the NNLP scale
	//
	LevelMethod string `json:"levelMethod" form:"levelMethod" query:"levelMethod"`
	//
	// parameter to support the level calculation
	// will typically be an NNLP progression level short reference but can also be a uri.
	//
	LevelProgLevel string `json:"levelProgLevel" form:"levelProgLevel" query:"levelProgLevel"`
	//
	// numeric score from the original assessment
	//
	AssessmentScore int `json:"assessmentScore" form:"assessmentScore" query:"assessmentScore"`
	//
	// if not a numeric score, then a textual judgegment such as 'mastered', 'acheived', or  a grade such as A-F
	//
	AssessmentToken string `json:"assessmentToken" form:"assessmentToken" query:"assessmentToken"`
}

//
// create a new service instance
//
func New(options ...Option) (*OtfLevelService, error) {

	srvc := OtfLevelService{}

	if err := srvc.setOptions(options...); err != nil {
		return nil, err
	}

	srvc.e = echo.New()
	srvc.e.Logger.SetLevel(log.INFO)
	// add pingable method to know we're up
	srvc.e.GET("/", func(c echo.Context) error {
		return c.JSON(http.StatusOK, "OK")
	})
	// add align method
	srvc.e.POST("/level", srvc.buildLevelHandler())

	return &srvc, nil
}

//
// start the service running
//
func (s *OtfLevelService) Start() {

	address := fmt.Sprintf("%s:%d", s.serviceHost, s.servicePort)
	go func(addr string) {
		if err := s.e.Start(addr); err != nil {
			s.e.Logger.Info("error starting server: ", err, ", shutting down...")
			// attempt clean shutdown by raising sig int
			p, _ := os.FindProcess(os.Getpid())
			p.Signal(os.Interrupt)
		}
	}(address)

}

//
// creates the main align method
// requires an input of request variables (in json)
// levelMethod: one of (prescribed|mapped)
// levelToken: NNLP progress level identifier (string or uri)
// assessmentScore:  original score from assessment system
//
func (s *OtfLevelService) buildLevelHandler() echo.HandlerFunc {

	niasURL := fmt.Sprintf("http://%s:%d/n3/graphql", s.niasHost, s.niasPort) // n3w address
	n3Token := s.niasToken
	sName := s.serviceName
	sID := s.serviceID

	return func(c echo.Context) error {
		// check required params are in input
		lr := &LevelRequest{}
		if err := c.Bind(lr); err != nil {
			fmt.Println("bind error: ", err)
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		// fmt.Printf("\tlevel-request: %+v\n\n", lr)

		if lr.LevelMethod == "" || lr.LevelProgLevel == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "must supply values for levelMethod & levelProgLevel")
		}

		// set default request headers
		headers := map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Connection":    "keep-alive",
			"DNT":           "1",
			"Authorization": n3Token,
		}

		result, err := calculateLevel(lr, niasURL, headers)
		if err != nil {
			fmt.Println("calculation error: ", err)
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}

		levelResponse := map[string]interface{}{
			"calculatedLevel": result,
			"levelMethod":     lr.LevelMethod,
			// "levelToken":          lr.LevelProgLevel,
			"assessmentScore":  lr.AssessmentScore,
			"assessmentToken":  lr.AssessmentToken,
			"levelServiceID":   sID,
			"levelServiceName": sName,
		}

		return c.JSON(http.StatusOK, levelResponse)

	}
}

//
// based on the levelrequest parameters use a mapping lookup
// to establish the national score for this assessment
//
// lr: the LevelRequest
// url: the url of the n3 service gql endpoint
// headers: outbound request headers
//
// returns: json object {progressionLevel: , score: }
//
func calculateLevel(lr *LevelRequest, url string, headers map[string]string) (map[string]interface{}, error) {

	method := "POST"
	body := bytes.NewBuffer(buildQuery(lr.LevelProgLevel))

	// call the n3 service to find the progression-level scale
	res, err := util.Fetch(method, url, headers, body)
	if err != nil {
		return nil, err
	}

	// extract the scale object from the response
	plScale := gjson.GetBytes(res, "data.q.OtfScale.0")
	if !plScale.Exists() {
		return nil, errors.New("no progress-level scale registered")
	}

	// extract the scale values

	// currently not needed
	// var low,high int64
	// low = plScale.Get("low").Int()
	// high = plScale.Get("high").Int()
	var achieved, partiallyAchieved, scaledScore int64
	var achievement string
	achieved = plScale.Get("achieved").Int()
	partiallyAchieved = plScale.Get("partiallyAchieved").Int()

	switch lr.LevelMethod {
	case "prescribed", "mapped":
		switch lr.AssessmentToken {
		case "mastered", "fully mastered":
			scaledScore = achieved
			achievement = "mastered"
		case "intermittent", "partial", "satisfied":
			scaledScore = partiallyAchieved
			achievement = "partially achieved"
		default:
			scaledScore = partiallyAchieved
			achievement = "partially achieved"
		}
	// to be done in backlog
	// case "mapped":
	// fetch the lookup
	// then calc against scale
	default:
		return nil, errors.New("levelMethod not supported")
	}

	result := map[string]interface{}{
		"achievement":      achievement,
		"progressionLevel": lr.LevelProgLevel,
		"scaledScore":      scaledScore,
	}

	return result, nil
}

//
// helper type to capture
// graphql queries for sending to
// the n3 service
//
type GQLQuery struct {
	Query     string
	Variables map[string]interface{}
}

//
// constructs the graphql query for
// mapped alignment requests
// token: the value to start searching from in n3
//
// returns: the byte array of the whole query request as json
//
func buildQuery(token string) []byte {

	// the data we want returned
	q := `query nlpLinksQuery($qspec: QueryInput!) { 
		q(qspec: $qspec) { 
	    	OtfScale {
      			partiallyAchieved
      			progressionLevel
      			scaleItemId
      			achieved
      			high
      			low
    		}
		}
	}`
	// the parameters of the query, defines staet-point and traversal in n3
	v := map[string]interface{}{
		"qspec": map[string]interface{}{
			"queryType":  "findByValue",
			"queryValue": token,
		},
	}

	gql := GQLQuery{Query: q, Variables: v}
	jsonStr, err := json.Marshal(gql)
	if err != nil {
		fmt.Println("gql query json marshal error: ", err)
	}

	return jsonStr
}

//
// shut the server down gracefully
//
func (s *OtfLevelService) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.e.Shutdown(ctx); err != nil {
		fmt.Println("could not shut down server cleanly: ", err)
		s.e.Logger.Fatal(err)
	}

}

func (s *OtfLevelService) PrintConfig() {

	fmt.Println("\n\tOTF-Level Service Configuration")
	fmt.Println("\t---------------------------------\n")

	s.printID()
	s.printNiasConfig()

}

func (s *OtfLevelService) printID() {
	fmt.Println("\tservice name:\t\t", s.serviceName)
	fmt.Println("\tservice ID:\t\t", s.serviceID)
	fmt.Println("\tservice host:\t\t", s.serviceHost)
	fmt.Println("\tservice port:\t\t", s.servicePort)
}

func (s *OtfLevelService) printNiasConfig() {
	fmt.Println("\tnias n3w host:\t\t", s.niasHost)
	fmt.Println("\tnias n3w port:\t\t", s.niasPort)
	// display only a partial token
	tokenParts := strings.Split(s.niasToken, ".")
	partialToken := tokenParts[len(tokenParts)-1]
	fmt.Println("\tn3w token(partial):\t", partialToken)
}
