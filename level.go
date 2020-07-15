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

	"github.com/iancoleman/strcase"
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
	// will typically be an NNLP progression level reference, can also be a uri.
	//
	LevelToken string `json:"levelToken" form:"levelToken" query:"levelToken"`
	//
	// score from the original assessment
	//
	AssessmentScore int `json:"assessmentScore" form:"assessmentScore" query:"assessmentScore"`
	//
	// if not a numeric score, then a textual judgegment such as 'mastered', 'acheived' etc.
	//
	AssessmentJudgement string `json:"assessmentJudgement" form:"assessmentJudgement" query:"assessmentJudgement"`
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

		fmt.Printf("\nlevel-request:%+v\n\n", lr)

		if lr.LevelMethod == "" || lr.LevelToken == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "must supply values for levelMethod & levelToken")
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
			"calculatedLevel":     result,
			"levelMethod":         lr.LevelMethod,
			"levelToken":          lr.LevelToken,
			"assessmentScore":     lr.AssessmentScore,
			"assessmentJudgement": lr.AssessmentJudgement,
			"levelServiceID":      sID,
			"levelServiceName":    sName,
		}

		return c.JSON(http.StatusOK, levelResponse)

	}
}

func calculateLevel(lr *LevelRequest, url string, headers map[string]string) (map[string]interface{}, error) {

	method := "POST"
	body := bytes.NewBuffer(buildQuery(lr.LevelToken))

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
	var low, high, achieved, partiallyAchieved, scaledScore int64
	low = plScale.Get("low").Int()
	high = plScale.Get("high").Int()
	_ = low
	_ = high
	achieved = plScale.Get("achieved").Int()
	partiallyAchieved = plScale.Get("partiallyAchieved").Int()

	switch lr.LevelMethod {
	case "prescribed":
		switch lr.AssessmentJudgement {
		case "mastered", "fully mastered":
			scaledScore = achieved
		case "intermittent", "partial", "satisfied":
			scaledScore = partiallyAchieved
		}
	case "mapped":
		// fetch the lookup
		// then calc against scale
	default:
		return nil, errors.New("levelMethod not supported")
	}

	result := map[string]interface{}{
		"method":           lr.LevelMethod,
		"progressionLevel": lr.LevelToken,
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
// ------
//

//
// calls the n3w server to find linked nlps
//
// token: the search token
// url: the url of the n3w server
// headers: http headers to support the request
//
// returns array of aligned nlp references
//
func mappedAlignment(token, url string, headers map[string]string) ([]string, error) {

	method := "POST"
	body := bytes.NewBuffer(buildQuery(token))

	// call the n3 service to find any nlp matches
	res, err := util.Fetch(method, url, headers, body)
	if err != nil {
		return nil, err
	}
	return extractN3AlignmentMatches(res), nil

}

//
// finds the aligned nlp identifiers from the results of an
// n3 (mapped) query.
//
// returns an arrray of identifiers, which can be empty
// if no matches were found
//
func extractN3AlignmentMatches(n3response []byte) []string {

	matches := make([]string, 0)

	result := gjson.GetBytes(n3response, "data.q.OtfNLPLink.#.nlpReference")
	for _, ref := range result.Array() {
		matches = append(matches, ref.String())
	}

	return matches

}

//
// calls the text-classfication server to find the
// nlp gesdi block for the specified token
//
// token: the search token
// url: the url of the text-class server
// headers: http headers to support the request
//
// returns array of aligned nlp objects (map[string]interface{} for conversion to json)
//
func prescribedAlignment(token, url string, headers map[string]string) ([]map[string]interface{}, error) {

	method := "GET"
	tcurl := fmt.Sprintf(`%s?search=%s`, url, token)
	// call the text-classfier lookup service
	res, err := util.Fetch(method, tcurl, headers, nil)
	if err != nil {
		return nil, err
	}
	return reformatClassifierLookupResponse(res)

}

//
// calls the text-classfication server to find the
// nlp gesdi block based on searching for best match to the
// supplied text (typically a phrase or description)
//
// token: the search token
// capability: text-class needs broad area (literacy/numeracy)
// url: the url of the text-class server
// headers: http headers to support the request
//
// returns array of aligned nlp objects (map[string]interface{} for conversion to json)
//

func inferredAlignment(token, capability, url string, headers map[string]string) ([]map[string]interface{}, error) {

	method := "POST"
	requestJson := []byte(fmt.Sprintf(`{"area":"%s", "text":%q}`, capability, token))
	body := bytes.NewReader(requestJson)
	// call the text classifier service
	res, err := util.Fetch(method, url, headers, body)
	if err != nil {
		return nil, err
	}
	return reformatClassifierResponse(res)

}

// //
// // constructs the graphql query for
// // mapped alignment requests
// // token: the value to start searching from in n3
// //
// // returns: the byte array of the whole query request as json
// //
// func buildQuery(token string) []byte {

// 	// the data we want returned
// 	q := `query nlpLinksQuery($qspec: QueryInput!) {
// 		q(qspec: $qspec) {
// 			OtfNLPLink {
// 				linkReference
// 				nlpNodeId
// 				nlpReference
// 				nlpLinkVersion
// 			}
// 			OtfProviderItem {
// 				providerName
// 				externalReference
// 				itemVersion
// 			}
// 		}
// 	}`
// 	// the parameters of the query, defines staet-point and traversal in n3
// 	v := map[string]interface{}{
// 		"qspec": map[string]interface{}{
// 			"queryType":  "traversalWithValue",
// 			"queryValue": token,
// 			"traversal":  []string{"OtfProviderItem", "OtfNLPLink"},
// 		},
// 	}

// 	gql := GQLQuery{Query: q, Variables: v}
// 	jsonStr, err := json.Marshal(gql)
// 	if err != nil {
// 		fmt.Println("gql query json marshal error: ", err)
// 	}

// 	return jsonStr
// }

//
// create the simplified return structure
// cr: payload returned by otf-classifier as bytes
//
// returns an array of nlp objects ([]map[string]interface{})
// to be converted to json
//
func reformatClassifierResponse(cr []byte) ([]map[string]interface{}, error) {

	// // return just first entry - highest match
	var clResp []map[string]interface{}
	err := json.Unmarshal(cr, &clResp)
	if err != nil {
		return nil, errors.Wrap(err, "unable to unmarshal response from classifier")
	}
	firstRec := clResp[0]

	alignments := []map[string]interface{}{}
	alignment := map[string]interface{}{
		"itemID":           firstRec["Item"],
		"developmentLevel": firstRec["DevLevel"],
		"itemText":         firstRec["Text"],
	}
	// convert paths array into object
	paths := firstRec["Path"].([]interface{})
	for _, path := range paths {
		p := path.(map[string]interface{})
		key := strcase.ToLowerCamel(p["Key"].(string)) // ensure keys work as json keys
		alignment[key] = p["Val"]
	}
	alignments = append(alignments, alignment)

	return alignments, nil
}

//
// create the simplified return structure
// cr: payload returned by otf-classifier as bytes
//
// returns an array of nlp objects ([]map[string]interface{})
// to be converted to json
//
func reformatClassifierLookupResponse(cr []byte) ([]map[string]interface{}, error) {

	var clResp []map[string]interface{}
	err := json.Unmarshal(cr, &clResp)
	if err != nil {
		return nil, errors.Wrap(err, "unable to unmarshal response from classifier lookup")
	}

	// convert paths array into object
	alignments := []map[string]interface{}{}
	alignment := map[string]interface{}{}
	for _, path := range clResp {
		p := path
		key := strcase.ToLowerCamel(p["Key"].(string)) // ensure keys work as json keys
		alignment[key] = p["Val"]
	}
	alignments = append(alignments, alignment)

	return alignments, nil

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
