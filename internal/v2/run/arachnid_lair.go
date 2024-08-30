package run

import (
	"github.com/hectorgimenez/d2go/pkg/data"
	"github.com/hectorgimenez/d2go/pkg/data/area"
	"github.com/hectorgimenez/koolo/internal/config"
	"github.com/hectorgimenez/koolo/internal/v2/action"
	"github.com/hectorgimenez/koolo/internal/v2/context"
)

type ArachnidLair struct {
	ctx *context.Status
}

func NewArachnidLair() *ArachnidLair {
	return &ArachnidLair{
		ctx: context.Get(),
	}
}

func (a ArachnidLair) Name() string {
	return string(config.ArachnidLairRun)
}

func (a ArachnidLair) Run() error {
	err := action.WayPoint(area.SpiderForest)
	if err != nil {
		return err
	}

	err = action.MoveToArea(area.SpiderCave)
	if err != nil {
		return err
	}

	action.OpenTPIfLeader()

	// Clear ArachnidLair
	return action.ClearCurrentLevel(true, data.MonsterAnyFilter())
}
