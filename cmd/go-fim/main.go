// Command go-fim is the file integrity manager agent.
//
//	go-fim [-c gofim.yml] [-v] [-local]    # cron-driven scan loop
//	go-fim [-c gofim.yml] --setup <jwt>        # one-shot registration handshake
//
// The setup handshake is one-time: the operator gets a JWT from the dashboard's
// "Add agent" form, runs go-fim with --setup once, then schedules the no-flag
// invocation via cron.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Jimzical/go-fim/internal/agent"
)

func main() {
	cfgPath := flag.String("c", "gofim.yml", "path to config file")
	verbose := flag.Bool("v", false, "force verbose (overrides config)")
	local := flag.Bool("local", false, "run without a config file, scanning cwd with local defaults (no server)")
	setupToken := flag.String("setup", "", "register this agent using the given JWT, then exit")
	flag.Parse()

	var err error
	if *setupToken != "" {
		err = agent.Setup(agent.SetupOpts{ConfigPath: *cfgPath, Token: *setupToken})
	} else {
		err = agent.Run(*cfgPath, *verbose, *local)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
