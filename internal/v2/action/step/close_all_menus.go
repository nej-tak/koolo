package step

import (
	"errors"

	"github.com/hectorgimenez/koolo/internal/v2/context"
	"github.com/hectorgimenez/koolo/internal/v2/utils"
	"github.com/lxn/win"
)

func CloseAllMenus() error {
	ctx := context.Get()
	ctx.ContextDebug.LastStep = "CloseAllMenus"

	attempts := 0
	for ctx.Data.OpenMenus.IsMenuOpen() {
		// Pause the execution if the priority is not the same as the execution priority
		ctx.PauseIfNotPriority()

		ctx.RefreshGameData()
		if attempts > 10 {
			return errors.New("failed closing game menu")
		}
		ctx.HID.PressKey(win.VK_ESCAPE)
		utils.Sleep(200)
		attempts++
	}

	return nil
}
