package executor

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/moconnor/pve-agent/internal/types"
)

// pgIDRegex validates PG ID format: pool_id.hex_pg_num (e.g., 4.3b)
var pgIDRegex = regexp.MustCompile(`^\d+\.[0-9a-fA-F]+$`)

func isValidPgID(pgID string) bool {
	return pgIDRegex.MatchString(pgID)
}

// executeCeph handles Ceph CLI commands
func (e *Executor) executeCeph(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	// 30 second timeout for ceph commands
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var args []string
	switch cmd.Action {
	case "ceph_pg_repair":
		pgID := cmd.Params["pg_id"].(string)
		args = []string{"pg", "repair", pgID}

	case "ceph_health_detail":
		args = []string{"health", "detail"}

	case "ceph_osd_tree":
		args = []string{"osd", "tree"}

	case "ceph_status":
		args = []string{"status"}

	case "ceph_pg_query":
		pgID := cmd.Params["pg_id"].(string)
		args = []string{"pg", pgID, "query"}

	default:
		result.Error = "unknown ceph command"
		return
	}

	execCmd := exec.CommandContext(ctx, "ceph", args...)
	output, err := execCmd.CombinedOutput()
	result.Output = strings.TrimSpace(string(output))

	if err != nil {
		result.Error = err.Error()
		return
	}

	result.Success = true
}
