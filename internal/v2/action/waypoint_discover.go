package action

import (
	"log/slog"

	"github.com/hectorgimenez/koolo/internal/v2/action/step"
	"github.com/hectorgimenez/koolo/internal/v2/context"
)

func DiscoverWaypoint() error {
	ctx := context.Get()
	ctx.ContextDebug.LastAction = "DiscoverWaypoint"

	ctx.Logger.Info("Trying to autodiscover Waypoint for current area", slog.String("area", ctx.Data.PlayerUnit.Area.Area().Name))
	for _, o := range ctx.Data.Objects {
		if o.IsWaypoint() {
			err := InteractObject(o, func() bool {
				return ctx.Data.OpenMenus.Waypoint
			})
			if err != nil {
				return err
			}

			ctx.Logger.Info("Waypoint discovered", slog.String("area", ctx.Data.PlayerUnit.Area.Area().Name))
			step.CloseAllMenus()
		}
	}

	ctx.Logger.Info("Waypoint not found :(", slog.String("area", ctx.Data.PlayerUnit.Area.Area().Name))
	return nil
}
