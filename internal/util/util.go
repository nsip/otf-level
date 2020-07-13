package util

import (
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/big"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/nats-io/nuid"
	"github.com/pkg/errors"
	hashids "github.com/speps/go-hashids"
)

var (
	once      sync.Once
	netClient *http.Client
)

//
// create a singleton http client to ensure
// maximum reuse of connection
//
func newNetClient() *http.Client {
	once.Do(func() {
		var netTransport = &http.Transport{
			Dial: (&net.Dialer{
				Timeout: 10 * time.Second,
			}).Dial,
			TLSHandshakeTimeout: 2 * time.Second,
		}
		netClient = &http.Client{
			Timeout:   time.Second * 2,
			Transport: netTransport,
		}
	})

	return netClient
}

//
// generate a short useful unique name - hashid in this case
//
func GenerateName() string {

	name := "aligner"

	// generate a random number
	number0, err := rand.Int(rand.Reader, big.NewInt(10000000))

	hd := hashids.NewData()
	hd.Salt = "otf-align random name generator 2020"
	hd.MinLength = 5
	h, err := hashids.NewWithData(hd)
	if err != nil {
		log.Println("error auto-generating name: ", err)
		return name
	}
	e, err := h.EncodeInt64([]int64{number0.Int64()})
	if err != nil {
		log.Println("error encoding auto-generated name: ", err)
		return name
	}
	name = e

	return name

}

//
// generate a unique id - nuid in this case
//
func GenerateID() string {

	return nuid.Next()

}

//
// Makes network calls to other services (text-class, nias), and returns
// the response payload as bytes, or an error
//
// method - http method to invoke (post/put/get etc.)
// header - map of headers to include in request
// body - reader for any content to supply as request body
//
func Fetch(method string, url string, header map[string]string, body io.Reader) ([]byte, error) {

	// Create request.
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	// //
	// // TODO: turn off in production
	// //
	// reqDump, err := httputil.DumpRequestOut(req, true)
	// if err != nil {
	// 	fmt.Println("req-dump error: ", err)
	// }
	// fmt.Printf("\nrequest\n\n%s\n\n", reqDump)

	// Add any required headers.
	for key, value := range header {
		req.Header.Add(key, value)
	}

	// Perform the network call.
	res, err := newNetClient().Do(req)
	if err != nil {
		return nil, err
	}

	// //
	// // TODO: turn off in production
	// //
	// responseDump, err := httputil.DumpResponse(res, true)
	// if err != nil {
	// 	fmt.Println("resp-dump error: ", err)
	// }
	// fmt.Printf("\nresponse:\n\n%s\n\n", responseDump)

	// If response from network call is not 200, return error.
	if res.StatusCode != http.StatusOK {
		return nil, errors.New(fmt.Sprintf("Network call failed with response: %d", res.StatusCode))
	}

	// return response payload as bytes
	respByte, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "cannot read Fetch response")
	}
	res.Body.Close()

	return respByte, nil
}

//
// small utility function embedded in major ops
// to print a performance indicator.
//
func TimeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Printf("%s took %s", name, elapsed.Truncate(time.Millisecond).String())

}

//
// find an available tcp port
//
func AvailablePort() (int, error) {

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, errors.Wrap(err, "cannot acquire a tcp port")
	}

	return listener.Addr().(*net.TCPAddr).Port, nil

}
