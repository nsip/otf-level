package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"

	otflvl "github.com/nsip/otf-level"
	"github.com/peterbourgon/ff"
)

func main() {

	fs := flag.NewFlagSet("otf-reader", flag.ExitOnError)
	var (
		_           = fs.String("config", "", "config file (optional), json format.")
		serviceName = fs.String("name", "", "name for this alignment service instance")
		serviceID   = fs.String("id", "", "id for this alignment service instance, leave blank to auto-generate a unique id")
		serviceHost = fs.String("host", "localhost", "name/address of host for this service")
		servicePort = fs.Int("port", 0, "port to run service on, if not specified will assign an available port automatically")
		niasHost    = fs.String("niasHost", "localhost", "host name/address of nias3 (n3w) web service")
		niasPort    = fs.Int("niasPort", 1323, "port that nias3 web (n3w) service is running on")
		niasToken   = fs.String("niasToken", "", "access token for nias server when making queries")
	)

	ff.Parse(fs, os.Args[1:],
		ff.WithConfigFileFlag("config"),
		ff.WithConfigFileParser(ff.JSONParser),
		ff.WithEnvVarPrefix("OTF_ALIGN_SRVC"),
	)

	opts := []otflvl.Option{
		otflvl.Name(*serviceName),
		otflvl.ID(*serviceID),
		otflvl.Host(*serviceHost),
		otflvl.Port(*servicePort),
		otflvl.NiasHost(*niasHost),
		otflvl.NiasPort(*niasPort),
		otflvl.NiasToken(*niasToken),
	}

	srvc, err := otflvl.New(opts...)
	if err != nil {
		fmt.Printf("\nCannot create otf-level service:\n%s\n\n", err)
		return
	}

	srvc.PrintConfig()

	// signal handler for shutdown
	closed := make(chan struct{})
	c := make(chan os.Signal)
	signal.Notify(c, os.Kill, os.Interrupt)
	go func() {
		<-c
		fmt.Println("\notf-level shutting down")
		srvc.Shutdown()
		fmt.Println("otf-align closed")
		close(closed)
	}()

	srvc.Start()

	// block until shutdown by sig-handler
	<-closed

}
