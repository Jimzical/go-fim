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
	"strconv"
	"time"

	"github.com/Jimzical/go-fim/internal/agent"
)

type durationFlag time.Duration

func (d *durationFlag) String() string {
	return time.Duration(*d).String()
}

func (d *durationFlag) Set(s string) error {
	v, err := time.ParseDuration(s)
	if err == nil {
		*d = durationFlag(v)
		return nil
	}
	if sec, err := strconv.Atoi(s); err == nil && sec >= 0 {
		*d = durationFlag(time.Duration(sec) * time.Second)
		return nil
	}
	return fmt.Errorf("invalid duration %q (use e.g. 30s or 30)", s)
}

func main() {
	cfgPath := flag.String("c", "gofim.yml", "path to config file")
	verbose := flag.Bool("v", false, "force verbose (overrides config)")
	local := flag.Bool("local", false, "scan cwd with local defaults when the config file is missing (no server)")
	setupToken := flag.String("setup", "", "register this agent using the given JWT, then exit")
	var jitter durationFlag
	flag.Var(&jitter, "jitter", "randomized schedule jitter to spread out request bursts (e.g. 30s)")
	flag.Parse()

	var err error
	if *setupToken != "" {
		err = agent.Setup(agent.SetupOpts{ConfigPath: *cfgPath, Token: *setupToken, Jitter: time.Duration(jitter)})
	} else {
		err = agent.Run(*cfgPath, *verbose, *local, time.Duration(jitter))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
