package action

import (
	"github.com/hectorgimenez/d2go/pkg/data"
	"github.com/hectorgimenez/koolo/internal/game"
	"github.com/hectorgimenez/koolo/internal/helper"
)

func (b *Builder) EnsureNoIlligalRejuvs() *Chain {
	/* @todo-nm
	add a greedy option for stashing illegal rejuvs
	instead of drinking them straight away */
	return NewChain(func(d game.Data) (actions []Action) {
		if !b.IsCarryingIllegalRejuvs(d) {
			return
		}

		b.Logger.Info("Carrying rejuv in a none-rejuv belt slot, drinking them and refilling pots.")

		_, _, rejuvsInBelt := b.bm.GetCurrentPotions(d)
		for i := 0; i < rejuvsInBelt; i++ {
			b.bm.DrinkPotion(d, data.RejuvenationPotion, false)
			helper.Sleep(100)
		}

		return append(actions,
			b.VendorRefill(true, false),
		)
	})
}

func (b *Builder) IsCarryingIllegalRejuvs(d game.Data) bool {
	rejuvsAllowed := b.CharacterCfg.Inventory.BeltColumns.Total(data.RejuvenationPotion) * d.Inventory.Belt.Rows()
	_, _, rejuvsInBelt := b.bm.GetCurrentPotions(d)

	if rejuvsAllowed > 0 {
		return false
	}

	if rejuvsAllowed == 0 && rejuvsInBelt > 0 {
		return true
	}

	return false
}
