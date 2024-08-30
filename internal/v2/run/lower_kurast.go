package run

import (
	"github.com/hectorgimenez/d2go/pkg/data"
	"github.com/hectorgimenez/d2go/pkg/data/area"
	"github.com/hectorgimenez/koolo/internal/config"
	"github.com/hectorgimenez/koolo/internal/v2/action"
	"github.com/hectorgimenez/koolo/internal/v2/context"
)

type LowerKurast struct {
	ctx *context.Status
}

func NewLowerKurast() *LowerKurast {
	return &LowerKurast{
		ctx: context.Get(),
	}
}

func (a LowerKurast) Name() string {
	return string(config.LowerKurastRun)
}

func (a LowerKurast) Run() error {

	// Use Waypoint to Lower Kurast
	err := action.WayPoint(area.LowerKurast)
	if err != nil {
		return err
	}

	// Clear Lower Kurast
	return action.ClearCurrentLevel(true, data.MonsterAnyFilter())

}
