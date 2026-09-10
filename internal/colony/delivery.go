package colony

import (
	"fmt"
	"strings"

	"github.com/russ-p/paseka/internal/forge"
	"github.com/russ-p/paseka/internal/gitroot"
)

// DiagnoseDelivery returns operator warnings when pull_request delivery cannot publish.
func DiagnoseDelivery(ctx Context, manifest Colony) []string {
	if manifest.Defaults.ResolvedDelivery() != DeliveryPullRequest {
		return nil
	}
	var warnings []string
	if len(ctx.Home.Forge.Command) == 0 {
		warnings = append(warnings, "defaults.delivery is pull_request but home forge.command is unset")
	} else if err := forge.CheckCommand(ctx.Home.Forge.Command, ctx.ColonyRoot); err != nil {
		warnings = append(warnings, fmt.Sprintf("defaults.delivery is pull_request but %s", err.Error()))
	}
	origin, err := gitroot.OriginURL(ctx.ColonyRoot)
	if err == nil && strings.TrimSpace(origin) == "" {
		warnings = append(warnings, "defaults.delivery is pull_request but the clone has no origin remote")
	}
	return warnings
}
