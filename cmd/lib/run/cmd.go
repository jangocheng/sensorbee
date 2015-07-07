// run package implements sensorbee's subcommand command which runs an API
// server.
package run

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"net/http"
	"os"
	"pfi/sensorbee/sensorbee/server"
	"strconv"
	"strings"
	"sync"
)

// SetUp sets up SensorBee's HTTP server. The URL or port ID is set with server
// configuration file, or command line arguments.
func SetUp() cli.Command {
	cmd := cli.Command{
		Name:        "run",
		Usage:       "run the server",
		Description: "run command starts a new server process",
		Action:      Run,
	}
	cmd.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "port",
			Value:  "8090",
			Usage:  "server port number",
			EnvVar: "PORT",
		},
	}
	return cmd
}

// Run run the HTTP server.
func Run(c *cli.Context) {

	defer func() {
		// This logic is provided to write test codes for this command line tool like below:
		// if v := recover(); v != nil {
		//   if testMode {
		//     testExitStatus = v.(int)
		//   } else {
		//     os.Exit(v.(int))
		//   }
		// }
		if v := recover(); v != nil {
			os.Exit(v.(int))
		}
	}()

	logger := logrus.New()
	// TODO: setup logger based on the config
	topologies := server.NewDefaultTopologyRegistry()

	root := server.SetUpRouter("/", server.ContextGlobalVariables{
		Logger:     logger,
		Topologies: topologies,
	})
	server.SetUpAPIRouter("/", root, nil)

	handler := func(rw http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			root.ServeHTTP(rw, r)
		}
	}

	port, err := strconv.Atoi(c.String("port"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cannot get port number:", err)
	}

	mutex := &sync.Mutex{}
	cond := sync.NewCond(mutex)
	var serverErr error

	mutex.Lock()
	defer mutex.Unlock()

	ports := []int{port} // TODO do need to have several port??
	for _, p := range ports {
		p := p // create a copy of the loop variable for the closure below

		go func() {
			// TODO: We need to listen first, and then serve on it.
			s := &http.Server{
				Addr:    fmt.Sprint(":", p), // TODO Support bind
				Handler: http.HandlerFunc(handler),
			}

			err := s.ListenAndServe()
			if err != nil {
				mutex.Lock()
				defer mutex.Unlock()
				serverErr = err
				cond.Signal()
			}
		}()
	}

	cond.Wait()
	if serverErr != nil {
		fmt.Fprintln(os.Stderr, "Cannot start the server:", serverErr)
	}
}