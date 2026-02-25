package btpcli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
)

// BtpCli represents a BTP CLI wrapper
type BtpCli struct {
	// Path to the btp CLI executable
	BtpPath string
	// Global account to use
	GlobalAccount string
}

// NewClient creates a new BTP CLI client
func NewClient(btpPath string) *BtpCli {
	if btpPath == "" {
		btpPath = "btp" // assumes btp is in PATH
	}
	return &BtpCli{
		BtpPath: btpPath,
	}
}

// Execute runs a btp CLI command and returns the output
func (c *BtpCli) Execute(ctx context.Context, args ...string) ([]byte, error) {
	logCommand(ctx, c.BtpPath, args...)
	cmd := exec.CommandContext(ctx, c.BtpPath, args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("btp CLI error: %w, output: %s", err, string(output))
	}

	return output, nil
}

// ExecuteJSON runs a command with --format json and unmarshals the result
func (c *BtpCli) ExecuteJSON(ctx context.Context, result interface{}, args ...string) error {
	// Add '--format json' flag in front of everything else
	args = append([]string{"--format", "json"}, args...)

	output, err := c.Execute(ctx, args...)
	if err != nil {
		return err
	}

	return json.Unmarshal(output, result)
}

func logCommand(ctx context.Context, command string, args ...string) {
	masked := make([]string, len(args))
	copy(masked, args)
	for i, arg := range masked {
		// Simple masking for sensitive flags
		if arg == "--password" || arg == "--user" || arg == "--jwt" {
			if i+1 < len(args) {
				masked[i+1] = "****"
			}
		}
	}
	slog.DebugContext(ctx, "Executing BTP CLI command", "name", command, "args", masked)
}
