package character

import (
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/hectorgimenez/koolo/internal/game"

	"github.com/hectorgimenez/d2go/pkg/data"
	"github.com/hectorgimenez/d2go/pkg/data/npc"
	"github.com/hectorgimenez/d2go/pkg/data/skill"
	"github.com/hectorgimenez/d2go/pkg/data/stat"
	"github.com/hectorgimenez/koolo/internal/v2/action/step"
)

const (
	paladinLevelingMaxAttacksLoop = 10
)

type PaladinLeveling struct {
	BaseCharacter
}

func (s PaladinLeveling) CheckKeyBindings() []skill.ID {
	requireKeybindings := []skill.ID{skill.TomeOfTownPortal}
	missingKeybindings := []skill.ID{}

	for _, cskill := range requireKeybindings {
		if _, found := s.data.KeyBindings.KeyBindingForSkill(cskill); !found {
			missingKeybindings = append(missingKeybindings, cskill)
		}
	}

	if len(missingKeybindings) > 0 {
		s.logger.Debug("There are missing required key bindings.", slog.Any("Bindings", missingKeybindings))
	}

	return missingKeybindings
}

func (s PaladinLeveling) KillMonsterSequence(
	monsterSelector func(d game.Data) (data.UnitID, bool),
	skipOnImmunities []stat.Resist,
) error {
	completedAttackLoops := 0
	previousUnitID := 0

	for {
		id, found := monsterSelector(*s.data)
		if !found {
			return nil
		}
		if previousUnitID != int(id) {
			completedAttackLoops = 0
		}

		if !s.preBattleChecks(id, skipOnImmunities) {
			return nil
		}

		if completedAttackLoops >= paladinLevelingMaxAttacksLoop {
			return nil
		}

		monster, found := s.data.Monsters.FindByID(id)
		if !found {
			s.logger.Info("Monster not found", slog.String("monster", fmt.Sprintf("%v", monster)))
			return nil
		}

		numOfAttacks := 5

		if s.data.PlayerUnit.Skills[skill.BlessedHammer].Level > 0 {
			s.logger.Debug("Using Blessed Hammer")
			// Add a random movement, maybe hammer is not hitting the target
			if previousUnitID == int(id) {
				if monster.Stats[stat.Life] > 0 {
					s.pf.RandomMovement(*s.data)
				}
				return nil
			}
			step.PrimaryAttack(id, numOfAttacks, false, step.Distance(2, 7), step.EnsureAura(skill.Concentration))

		} else {
			if s.data.PlayerUnit.Skills[skill.Zeal].Level > 0 {
				s.logger.Debug("Using Zeal")
				numOfAttacks = 1
			}
			s.logger.Debug("Using primary attack with Holy Fire aura")
			step.PrimaryAttack(id, numOfAttacks, false, step.Distance(1, 3), step.EnsureAura(skill.HolyFire))
		}

		completedAttackLoops++
		previousUnitID = int(id)
	}
}

func (s PaladinLeveling) killMonster(npc npc.ID, t data.MonsterType) error {
	return s.KillMonsterSequence(func(d game.Data) (data.UnitID, bool) {
		m, found := d.Monsters.FindOne(npc, t)
		if !found {
			return 0, false
		}

		return m.UnitID, true
	}, nil)
}

func (s PaladinLeveling) BuffSkills() []skill.ID {
	skillsList := make([]skill.ID, 0)
	if _, found := s.data.KeyBindings.KeyBindingForSkill(skill.HolyShield); found {
		skillsList = append(skillsList, skill.HolyShield)
	}
	s.logger.Info("Buff skills", "skills", skillsList)
	return skillsList
}

func (s PaladinLeveling) PreCTABuffSkills() []skill.ID {
	return []skill.ID{}
}

func (s PaladinLeveling) ShouldResetSkills() bool {
	lvl, _ := s.data.PlayerUnit.FindStat(stat.Level, 0)
	if lvl.Value >= 21 && s.data.PlayerUnit.Skills[skill.HolyFire].Level > 10 {
		s.logger.Info("Resetting skills: Level 21+ and Holy Fire level > 10")
		return true
	}

	return false
}

func (s PaladinLeveling) SkillsToBind() (skill.ID, []skill.ID) {
	lvl, _ := s.data.PlayerUnit.FindStat(stat.Level, 0)
	mainSkill := skill.AttackSkill
	skillBindings := []skill.ID{}

	if lvl.Value >= 6 {
		skillBindings = append(skillBindings, skill.Vigor)
	}

	if lvl.Value >= 24 {
		skillBindings = append(skillBindings, skill.HolyShield)
	}

	if s.data.PlayerUnit.Skills[skill.BlessedHammer].Level > 0 && lvl.Value >= 18 {
		mainSkill = skill.BlessedHammer
	} else if s.data.PlayerUnit.Skills[skill.Zeal].Level > 0 {
		mainSkill = skill.Zeal
	}

	if s.data.PlayerUnit.Skills[skill.Concentration].Level > 0 && lvl.Value >= 18 {
		skillBindings = append(skillBindings, skill.Concentration)
	} else {
		if _, found := s.data.PlayerUnit.Skills[skill.HolyFire]; found {
			skillBindings = append(skillBindings, skill.HolyFire)
		} else if _, found := s.data.PlayerUnit.Skills[skill.Might]; found {
			skillBindings = append(skillBindings, skill.Might)
		}
	}

	s.logger.Info("Skills bound", "mainSkill", mainSkill, "skillBindings", skillBindings)
	return mainSkill, skillBindings
}

func (s PaladinLeveling) StatPoints() map[stat.ID]int {
	lvl, _ := s.data.PlayerUnit.FindStat(stat.Level, 0)
	statPoints := make(map[stat.ID]int)

	if lvl.Value < 21 {
		statPoints[stat.Strength] = 0
		statPoints[stat.Dexterity] = 25
		statPoints[stat.Vitality] = 150
		statPoints[stat.Energy] = 0
	} else if lvl.Value < 30 {
		statPoints[stat.Strength] = 35
		statPoints[stat.Vitality] = 200
		statPoints[stat.Energy] = 0
	} else if lvl.Value < 45 {
		statPoints[stat.Strength] = 50
		statPoints[stat.Dexterity] = 40
		statPoints[stat.Vitality] = 220
		statPoints[stat.Energy] = 0
	} else {
		statPoints[stat.Strength] = 86
		statPoints[stat.Dexterity] = 50
		statPoints[stat.Vitality] = 300
		statPoints[stat.Energy] = 0
	}

	s.logger.Info("Assigning stat points", "level", lvl.Value, "statPoints", statPoints)
	return statPoints
}

func (s PaladinLeveling) SkillPoints() []skill.ID {
	lvl, _ := s.data.PlayerUnit.FindStat(stat.Level, 0)
	var skillPoints []skill.ID

	if lvl.Value < 21 {
		skillPoints = []skill.ID{
			skill.Might,
			skill.Sacrifice,
			skill.ResistFire,
			skill.ResistFire,
			skill.ResistFire,
			skill.HolyFire,
			skill.HolyFire,
			skill.HolyFire,
			skill.HolyFire,
			skill.HolyFire,
			skill.HolyFire,
			skill.Zeal,
			skill.HolyFire,
			skill.HolyFire,
			skill.HolyFire,
			skill.HolyFire,
			skill.HolyFire,
			skill.HolyFire,
			skill.HolyFire,
			skill.HolyFire,
			skill.HolyFire,
			skill.HolyFire,
		}
	} else {
		// Hammerdin
		skillPoints = []skill.ID{
			skill.HolyBolt,
			skill.BlessedHammer,
			skill.Prayer,
			skill.Defiance,
			skill.Cleansing,
			skill.Vigor,
			skill.Might,
			skill.BlessedAim,
			skill.Concentration,
			skill.BlessedAim,
			skill.BlessedAim,
			skill.BlessedAim,
			skill.BlessedAim,
			skill.BlessedAim,
			skill.BlessedAim,
			// Level 19
			skill.BlessedHammer,
			skill.Concentration,
			skill.Vigor,
			// Level 20
			skill.BlessedHammer,
			skill.Vigor,
			skill.BlessedHammer,
			skill.BlessedHammer,
			skill.BlessedHammer,
			skill.Vigor,
			skill.BlessedHammer,
			skill.BlessedHammer,
			skill.BlessedHammer,
			skill.BlessedHammer,
			skill.BlessedHammer,
			skill.BlessedHammer,
			skill.BlessedHammer,
			skill.BlessedHammer,
			skill.Smite,
			skill.BlessedHammer,
			skill.BlessedHammer,
			skill.BlessedHammer,
			skill.BlessedHammer,
			skill.BlessedHammer,
			skill.Charge,
			skill.BlessedHammer,
			skill.Vigor,
			skill.Vigor,
			skill.Vigor,
			skill.HolyShield,
			skill.Concentration,
			skill.Vigor,
			skill.Vigor,
			skill.Vigor,
			skill.Vigor,
			skill.Vigor,
			skill.Vigor,
			skill.Vigor,
			skill.Vigor,
			skill.Vigor,
			skill.Vigor,
			skill.Vigor,
			skill.Vigor,
			skill.Vigor,
			skill.Concentration,
			skill.Concentration,
			skill.Concentration,
			skill.Concentration,
			skill.Concentration,
			skill.Concentration,
			skill.Concentration,
			skill.Concentration,
			skill.Concentration,
			skill.Concentration,
			skill.Concentration,
			skill.Concentration,
			skill.Concentration,
			skill.Concentration,
			skill.Concentration,
			skill.BlessedAim,
			skill.BlessedAim,
			skill.BlessedAim,
			skill.BlessedAim,
			skill.BlessedAim,
			skill.BlessedAim,
			skill.BlessedAim,
			skill.BlessedAim,
			skill.BlessedAim,
			skill.BlessedAim,
			skill.BlessedAim,
			skill.BlessedAim,
		}
	}

	s.logger.Info("Assigning skill points", "level", lvl.Value, "skillPoints", skillPoints)
	return skillPoints
}

func (s PaladinLeveling) KillCountess() error {
	return s.killMonster(npc.DarkStalker, data.MonsterTypeSuperUnique)
}

func (s PaladinLeveling) KillAndariel() error {
	return s.killMonster(npc.Andariel, data.MonsterTypeNone)
}

func (s PaladinLeveling) KillSummoner() error {
	return s.killMonster(npc.Summoner, data.MonsterTypeNone)
}

func (s PaladinLeveling) KillDuriel() error {
	return s.killMonster(npc.Duriel, data.MonsterTypeNone)
}

func (s PaladinLeveling) KillCouncil() error {
	return s.KillMonsterSequence(func(d game.Data) (data.UnitID, bool) {
		var councilMembers []data.Monster
		for _, m := range d.Monsters {
			if m.Name == npc.CouncilMember || m.Name == npc.CouncilMember2 || m.Name == npc.CouncilMember3 {
				councilMembers = append(councilMembers, m)
			}
		}

		// Order council members by distance
		sort.Slice(councilMembers, func(i, j int) bool {
			distanceI := s.pf.DistanceFromMe(councilMembers[i].Position)
			distanceJ := s.pf.DistanceFromMe(councilMembers[j].Position)

			return distanceI < distanceJ
		})

		if len(councilMembers) > 0 {
			s.logger.Debug("Targeting Council member", "id", councilMembers[0].UnitID)
			return councilMembers[0].UnitID, true
		}

		s.logger.Debug("No Council members found")
		return 0, false
	}, nil)
}

func (s PaladinLeveling) KillMephisto() error {
	return s.killMonster(npc.Mephisto, data.MonsterTypeNone)
}

func (s PaladinLeveling) KillIzual() error {
	return s.killMonster(npc.Izual, data.MonsterTypeNone)
}

func (s PaladinLeveling) KillDiablo() error {
	timeout := time.Second * 20
	startTime := time.Now()
	diabloFound := false

	for {
		if time.Since(startTime) > timeout && !diabloFound {
			s.logger.Error("Diablo was not found, timeout reached")
			return nil
		}

		diablo, found := s.data.Monsters.FindOne(npc.Diablo, data.MonsterTypeNone)
		if !found || diablo.Stats[stat.Life] <= 0 {
			// Already dead
			if diabloFound {
				return nil
			}

			// Keep waiting...
			time.Sleep(200)
			continue
		}

		diabloFound = true
		s.logger.Info("Diablo detected, attacking")

		s.killMonster(npc.Diablo, data.MonsterTypeNone)
		s.killMonster(npc.Diablo, data.MonsterTypeNone)

		return s.killMonster(npc.Diablo, data.MonsterTypeNone)
	}
}

func (s PaladinLeveling) KillPindle() error {
	return s.killMonster(npc.DefiledWarrior, data.MonsterTypeSuperUnique)
}

func (s PaladinLeveling) KillNihlathak() error {
	return s.killMonster(npc.Nihlathak, data.MonsterTypeSuperUnique)
}

func (s PaladinLeveling) KillAncients() error {
	for _, m := range s.data.Monsters.Enemies(data.MonsterEliteFilter()) {
		m, _ := s.data.Monsters.FindOne(m.Name, data.MonsterTypeSuperUnique)

		s.killMonster(m.Name, data.MonsterTypeSuperUnique)
	}
	return nil
}

func (s PaladinLeveling) KillBaal() error {
	return s.killMonster(npc.BaalCrab, data.MonsterTypeNone)
}
