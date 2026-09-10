package colony

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func applySelectedProfile(ctx *Context, manifest Colony, name string) error {
	colonyNames, homeNames, err := ListProfileNames(ctx.ColonyRoot, ctx.Slug)
	if err != nil {
		return err
	}
	colonyOK := colonyProfileExists(ctx.ColonyRoot, name)
	homeOK := homeProfilePresent(ctx.Slug, name)
	if !colonyOK && !homeOK {
		return fmt.Errorf("colony: profile %q not found (%s)", name, formatAvailableProfiles(colonyNames, homeNames))
	}

	var colonyOverlay ColonyProfile
	if colonyOK {
		overlay, loadErr := loadColonyProfileFile(colonyProfilePath(ctx.ColonyRoot, name))
		if loadErr != nil {
			return loadErr
		}
		if err := validateColonyProfileRoles(ctx.ColonyRoot, overlay); err != nil {
			return err
		}
		colonyOverlay = overlay
		ctx.ProfileLayers.Colony = true
	}

	var homeOverlay HomeProfileConfig
	if homeOK {
		cfgPath := ""
		if dir, dirErr := homeProfileDir(ctx.Slug, name); dirErr == nil {
			cfgPath = filepath.Join(dir, "config.yaml")
		}
		if fileExists(cfgPath) {
			cfg, loadErr := loadHomeProfileConfig(cfgPath)
			if loadErr != nil {
				return loadErr
			}
			homeOverlay = cfg
		}
		ctx.ProfileLayers.Home = true
	}

	ctx.Profile = name
	ctx.colonyProfile = colonyOverlay

	merged := MergedModelAliases(manifest.ModelAliases, colonyOverlay.ModelAliases)
	merged = MergedModelAliases(merged, ctx.Home.ModelAliases)
	merged = MergedModelAliases(merged, homeOverlay.ModelAliases)
	if err := ValidateModelAliases(merged); err != nil {
		return err
	}
	ctx.ModelAliases = merged

	if u := strings.TrimSpace(homeOverlay.NATS.URL); u != "" {
		ctx.Home.NATS.URL = u
	}

	dir, err := homeProfileDir(ctx.Slug, name)
	if err != nil {
		return err
	}
	adaptersDir := filepath.Join(dir, "adapters")
	cursor, err := overlayCursorAdapter(ctx.Cursor, filepath.Join(adaptersDir, "cursor.yaml"))
	if err != nil {
		return err
	}
	ctx.Cursor = cursor
	pi, err := overlayPiAdapter(ctx.Pi, filepath.Join(adaptersDir, "pi.yaml"))
	if err != nil {
		return err
	}
	ctx.Pi = pi
	claude, err := overlayClaudeAdapter(ctx.Claude, filepath.Join(adaptersDir, "claude.yaml"))
	if err != nil {
		return err
	}
	ctx.Claude = claude
	opencode, err := overlayOpenCodeAdapter(ctx.OpenCode, filepath.Join(adaptersDir, "opencode.yaml"))
	if err != nil {
		return err
	}
	ctx.OpenCode = opencode
	return nil
}

func validateColonyProfileRoles(colonyRoot string, overlay ColonyProfile) error {
	if len(overlay.Bees) == 0 {
		return nil
	}
	bees, err := LoadAllBees(colonyRoot)
	if err != nil {
		return err
	}
	for role := range overlay.Bees {
		if _, ok := bees[role]; !ok {
			return fmt.Errorf("colony: profile bees.%s: role not found", role)
		}
	}
	return nil
}

func overlayCursorAdapter(base CursorAdapterConfig, path string) (CursorAdapterConfig, error) {
	over, ok, err := readYAMLOverlay[CursorAdapterConfig](path)
	if err != nil || !ok {
		return base, err
	}
	if strings.TrimSpace(over.Binary) != "" {
		base.Binary = over.Binary
	}
	if strings.TrimSpace(over.APIKeyEnv) != "" {
		base.APIKeyEnv = over.APIKeyEnv
	}
	return base, nil
}

func overlayPiAdapter(base PiAdapterConfig, path string) (PiAdapterConfig, error) {
	over, ok, err := readYAMLOverlay[PiAdapterConfig](path)
	if err != nil || !ok {
		return base, err
	}
	if strings.TrimSpace(over.Binary) != "" {
		base.Binary = over.Binary
	}
	if strings.TrimSpace(over.APIKeyEnv) != "" {
		base.APIKeyEnv = over.APIKeyEnv
	}
	return base, nil
}

func overlayClaudeAdapter(base ClaudeAdapterConfig, path string) (ClaudeAdapterConfig, error) {
	over, ok, err := readYAMLOverlay[ClaudeAdapterConfig](path)
	if err != nil || !ok {
		return base, err
	}
	if strings.TrimSpace(over.Binary) != "" {
		base.Binary = over.Binary
	}
	if strings.TrimSpace(over.APIKeyEnv) != "" {
		base.APIKeyEnv = over.APIKeyEnv
	}
	return base, nil
}

func overlayOpenCodeAdapter(base OpenCodeAdapterConfig, path string) (OpenCodeAdapterConfig, error) {
	over, ok, err := readYAMLOverlay[OpenCodeAdapterConfig](path)
	if err != nil || !ok {
		return base, err
	}
	if strings.TrimSpace(over.Binary) != "" {
		base.Binary = over.Binary
	}
	return base, nil
}

func readYAMLOverlay[T any](path string) (T, bool, error) {
	var zero T
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return zero, false, nil
		}
		return zero, false, fmt.Errorf("colony: read profile %s: %w", path, err)
	}
	var over T
	if err := yaml.Unmarshal(data, &over); err != nil {
		return zero, false, fmt.Errorf("colony: parse profile %s: %w", path, err)
	}
	return over, true, nil
}

func mergeParamMaps(base, over map[string]any) map[string]any {
	if len(base) == 0 && len(over) == 0 {
		return nil
	}
	out := maps.Clone(base)
	if out == nil {
		out = map[string]any{}
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

// ApplyBeeProfile remaps adapter/params for one committed bee. No-op when no profile is selected.
func (c Context) ApplyBeeProfile(bee Bee) (Bee, ProfileApplyInfo, error) {
	info := ProfileApplyInfo{Role: bee.Role}
	if c.Profile == "" {
		return bee, info, nil
	}
	overlay := c.colonyProfile
	committedName, err := bee.ResolveAdapter()
	if err != nil {
		return bee, info, err
	}
	perBee, hasPerBee := overlay.Bees[bee.Role]
	perBeeAdapter := ""
	if hasPerBee {
		perBeeAdapter = strings.TrimSpace(perBee.Adapter)
	}

	global := strings.TrimSpace(overlay.Adapter)
	globalEligible := global != "" && isLLMAdapterName(committedName) && !bee.Command.IsSet()

	switch {
	case perBeeAdapter != "":
		bee.Adapter = perBeeAdapter
		info.Remapped = perBeeAdapter != committedName
	case globalEligible:
		bee.Adapter = global
		info.Remapped = global != committedName
	default:
		if global != "" {
			if committedName == "script" {
				info.SkippedScript = true
			} else if bee.Command.IsSet() {
				info.SkippedCommand = true
			}
		}
	}

	if global != "" && isLLMAdapterName(committedName) && !bee.Command.IsSet() {
		bee.Params = mergeParamMaps(bee.Params, overlay.Params)
	}
	if hasPerBee && len(perBee.Params) > 0 {
		bee.Params = mergeParamMaps(bee.Params, perBee.Params)
	}
	if _, err := bee.ResolveAdapter(); err != nil {
		return bee, info, err
	}
	return bee, info, nil
}

func applyBeesProfile(ctx Context, bees map[string]Bee) (map[string]Bee, error) {
	if ctx.Profile == "" {
		return bees, nil
	}
	out := make(map[string]Bee, len(bees))
	for role, bee := range bees {
		applied, _, err := ctx.ApplyBeeProfile(bee)
		if err != nil {
			return nil, err
		}
		out[role] = applied
	}
	return out, nil
}

// LoadBee reads committed YAML then applies the process profile overlay.
func (c Context) LoadBee(role string) (Bee, BeeLocalOverlay, error) {
	bee, overlay, err := LoadBee(c.ColonyRoot, role)
	if err != nil {
		return Bee{}, BeeLocalOverlay{}, err
	}
	bee, _, err = c.ApplyBeeProfile(bee)
	if err != nil {
		return Bee{}, BeeLocalOverlay{}, err
	}
	return bee, overlay, nil
}

// LoadAllBees returns committed bees with the process profile applied.
func (c Context) LoadAllBees() (map[string]Bee, error) {
	bees, err := LoadAllBees(c.ColonyRoot)
	if err != nil {
		return nil, err
	}
	return applyBeesProfile(c, bees)
}

// LoadAllBeesForDiagnosis returns diagnosis bees with the process profile applied.
func (c Context) LoadAllBeesForDiagnosis() (map[string]Bee, error) {
	bees, err := LoadAllBeesForDiagnosis(c.ColonyRoot)
	if err != nil {
		return nil, err
	}
	return applyBeesProfile(c, bees)
}

// ProfileDoctorView is operator-facing profile status for paseka doctor.
type ProfileDoctorView struct {
	Name            string
	Layers          ProfileLayers
	Remapped        []string
	SkippedScript   []string
	SkippedCommand  []string
	ColonyAvailable []string
	HomeAvailable   []string
}

// DoctorProfileView loads bees and reports remap/skip for the selected profile.
func (c Context) DoctorProfileView() (ProfileDoctorView, error) {
	view := ProfileDoctorView{
		Name:   c.Profile,
		Layers: c.ProfileLayers,
	}
	colonyNames, homeNames, err := ListProfileNames(c.ColonyRoot, c.Slug)
	if err != nil {
		return view, err
	}
	view.ColonyAvailable = colonyNames
	view.HomeAvailable = homeNames
	if c.Profile == "" {
		return view, nil
	}
	bees, err := LoadAllBeesForDiagnosis(c.ColonyRoot)
	if err != nil {
		return view, err
	}
	for _, role := range sortedBeeRoles(bees) {
		_, info, err := c.ApplyBeeProfile(bees[role])
		if err != nil {
			return view, err
		}
		if info.Remapped {
			view.Remapped = append(view.Remapped, role)
		}
		if info.SkippedScript {
			view.SkippedScript = append(view.SkippedScript, role)
		}
		if info.SkippedCommand {
			view.SkippedCommand = append(view.SkippedCommand, role)
		}
	}
	sort.Strings(view.Remapped)
	sort.Strings(view.SkippedScript)
	sort.Strings(view.SkippedCommand)
	return view, nil
}
