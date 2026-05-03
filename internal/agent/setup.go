package agent

import (
	"errors"
	"fmt"

	"github.com/Jimzical/go-fim/internal/client"
	"github.com/Jimzical/go-fim/internal/config"
	"github.com/Jimzical/go-fim/internal/logger"
	"github.com/Jimzical/go-fim/internal/store"
)

// SetupOpts is the parsed input for the setup subcommand.
type SetupOpts struct {
	ConfigPath string
	Token      string
}

// Setup runs the one-shot registration handshake: calls POST /api/setup with
// the JWT to register this agent's bbolt UUID with the server.
func Setup(opts SetupOpts) (err error) {
	if opts.Token == "" {
		return errors.New("setup: --setup is required")
	}

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}
	if cfg.ServerURL == "" {
		return errors.New("setup: server_url is empty in config — setup needs a control plane to talk to")
	}

	log := logger.New(false)

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer deferClose(db, &err)()

	agentID := cfg.AgentID
	if agentID == "" {
		agentID, err = store.AgentID(db)
		if err != nil {
			return err
		}
	}

	c := client.New(cfg.ServerURL, "", cfg.InsecureSkipVerify)
	resp, err := c.RegisterAgent(opts.Token, agentID)
	if err != nil {
		// 409 from the server means "this agent_id is already registered" —
		// we treat that as success so re-running setup is idempotent.
		var httpErr *client.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == 409 {
			log.Info("already registered", "agent_id", agentID)
			return nil
		}
		return fmt.Errorf("register: %w", err)
	}

	if err := store.SaveAPIToken(db, resp.APIToken); err != nil {
		return fmt.Errorf("save api token: %w", err)
	}

	log.Info("registered",
		"agent_id", resp.AgentID,
		"agent_name", resp.AgentName,
		"scan_path", resp.ScanPath,
	)
	return nil
}
