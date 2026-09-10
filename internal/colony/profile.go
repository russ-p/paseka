package colony

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const envProfile = "PASEKA_PROFILE"

var (
	processProfileMu sync.Mutex
	processProfile   ProfileSelection
)

// ProfileSelection is the Queen Shell process overlay choice (flags).
type ProfileSelection struct {
	NoProfile bool
	Name      string
	FlagSet   bool
}

// SetProcessProfile stores the root-flag selection for ResolveContext.
func SetProcessProfile(sel ProfileSelection) {
	processProfileMu.Lock()
	defer processProfileMu.Unlock()
	processProfile = sel
}

// ProcessProfile returns the current process overlay selection.
func ProcessProfile() ProfileSelection {
	processProfileMu.Lock()
	defer processProfileMu.Unlock()
	return processProfile
}

// ProfileLayers records which profile files loaded for the process.
type ProfileLayers struct {
	Colony bool
	Home   bool
}

// ColonyProfile is the committed overlay schema (.paseka/profiles/<name>.yaml).
type ColonyProfile struct {
	Adapter      string
	Params       map[string]any
	ModelAliases map[string]string
	Bees         map[string]BeeProfileOverlay
}

// BeeProfileOverlay is a per-role adapter/params exception.
type BeeProfileOverlay struct {
	Adapter string
	Params  map[string]any
}

// HomeProfileConfig overlays nats and model_aliases from a home profile.
type HomeProfileConfig struct {
	NATS         NATSConfig        `yaml:"nats"`
	ModelAliases map[string]string `yaml:"model_aliases"`
}

// ProfileApplyInfo is per-bee remap/skip result for doctor.
type ProfileApplyInfo struct {
	Role           string
	Remapped       bool
	SkippedScript  bool
	SkippedCommand bool
}

// ValidateProfileName rejects empty, dotted, and path-escaping profile names.
func ValidateProfileName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("colony: profile name is required")
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("colony: invalid profile name %q", name)
	}
	if strings.Contains(name, "/") || strings.Contains(name, `\`) || strings.Contains(name, "..") {
		return fmt.Errorf("colony: invalid profile name %q", name)
	}
	return nil
}

func isLLMAdapterName(name string) bool {
	switch name {
	case "cursor", "pi", "claude", "opencode":
		return true
	default:
		return false
	}
}

func validateProfileAdapter(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if name == "script" {
		return fmt.Errorf("colony: profile adapter %q is not allowed", name)
	}
	if !isLLMAdapterName(name) {
		return fmt.Errorf("colony: unknown adapter %q", name)
	}
	return nil
}

func selectedProfileName(sel ProfileSelection, sticky string) (string, error) {
	if sel.NoProfile {
		return "", nil
	}
	if sel.FlagSet {
		if err := ValidateProfileName(sel.Name); err != nil {
			return "", err
		}
		return strings.TrimSpace(sel.Name), nil
	}
	if env := strings.TrimSpace(os.Getenv(envProfile)); env != "" {
		if err := ValidateProfileName(env); err != nil {
			return "", err
		}
		return env, nil
	}
	sticky = strings.TrimSpace(sticky)
	if sticky == "" {
		return "", nil
	}
	if err := ValidateProfileName(sticky); err != nil {
		return "", fmt.Errorf("colony: home sticky profile: %w", err)
	}
	return sticky, nil
}

func colonyProfilesDir(colonyRoot string) string {
	return PasekaPath(colonyRoot, "profiles")
}

func colonyProfilePath(colonyRoot, name string) string {
	return filepath.Join(colonyProfilesDir(colonyRoot), name+".yaml")
}

func homeProfileDir(slug, name string) (string, error) {
	home, err := HomeDir(slug)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "profiles", name), nil
}

func colonyProfileExists(colonyRoot, name string) bool {
	_, err := os.Stat(colonyProfilePath(colonyRoot, name))
	return err == nil
}

func homeProfilePresent(slug, name string) bool {
	dir, err := homeProfileDir(slug, name)
	if err != nil {
		return false
	}
	if fileExists(filepath.Join(dir, "config.yaml")) {
		return true
	}
	adaptersDir := filepath.Join(dir, "adapters")
	entries, err := os.ReadDir(adaptersDir)
	if err != nil {
		return false
	}
	for _, ent := range entries {
		if !ent.IsDir() && strings.HasSuffix(ent.Name(), ".yaml") {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// ListProfileNames returns discovered colony file stems and loadable home profile dirs.
func ListProfileNames(colonyRoot, slug string) (colonyNames, homeNames []string, err error) {
	dir := colonyProfilesDir(colonyRoot)
	entries, readErr := os.ReadDir(dir)
	if readErr != nil && !os.IsNotExist(readErr) {
		return nil, nil, fmt.Errorf("colony: list profiles: %w", readErr)
	}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		stem := strings.TrimSuffix(name, ".yaml")
		if err := ValidateProfileName(stem); err != nil {
			continue
		}
		colonyNames = append(colonyNames, stem)
	}
	sort.Strings(colonyNames)

	if slug != "" {
		home, homeErr := HomeDir(slug)
		if homeErr != nil {
			return colonyNames, nil, homeErr
		}
		profilesDir := filepath.Join(home, "profiles")
		homeEntries, homeReadErr := os.ReadDir(profilesDir)
		if homeReadErr != nil && !os.IsNotExist(homeReadErr) {
			return colonyNames, nil, fmt.Errorf("colony: list home profiles: %w", homeReadErr)
		}
		for _, ent := range homeEntries {
			if !ent.IsDir() {
				continue
			}
			stem := ent.Name()
			if err := ValidateProfileName(stem); err != nil {
				continue
			}
			if homeProfilePresent(slug, stem) {
				homeNames = append(homeNames, stem)
			}
		}
		sort.Strings(homeNames)
	}
	return colonyNames, homeNames, nil
}

func formatAvailableProfiles(colonyNames, homeNames []string) string {
	colonyPart := "none"
	if len(colonyNames) > 0 {
		colonyPart = strings.Join(colonyNames, ", ")
	}
	homePart := "none"
	if len(homeNames) > 0 {
		homePart = strings.Join(homeNames, ", ")
	}
	return fmt.Sprintf("colony: %s; home: %s", colonyPart, homePart)
}

func loadColonyProfileFile(path string) (ColonyProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ColonyProfile{}, fmt.Errorf("colony: read profile %s: %w", path, err)
	}
	keys, err := yamlTopLevelKeys(data)
	if err != nil {
		return ColonyProfile{}, fmt.Errorf("colony: parse profile %s: %w", path, err)
	}
	allowed := map[string]struct{}{
		"adapter":       {},
		"params":        {},
		"model_aliases": {},
		"bees":          {},
	}
	for _, k := range keys {
		if _, ok := allowed[k]; !ok {
			return ColonyProfile{}, fmt.Errorf("colony: profile %s: forbidden key %q", path, k)
		}
	}

	var raw struct {
		Adapter      string                    `yaml:"adapter"`
		Params       map[string]any            `yaml:"params"`
		ModelAliases map[string]string         `yaml:"model_aliases"`
		Bees         map[string]map[string]any `yaml:"bees"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return ColonyProfile{}, fmt.Errorf("colony: parse profile %s: %w", path, err)
	}
	if err := validateProfileAdapter(raw.Adapter); err != nil {
		return ColonyProfile{}, fmt.Errorf("colony: profile %s: %w", path, err)
	}

	out := ColonyProfile{
		Adapter:      strings.TrimSpace(raw.Adapter),
		Params:       raw.Params,
		ModelAliases: NormalizeModelAliases(raw.ModelAliases),
		Bees:         map[string]BeeProfileOverlay{},
	}
	if err := ValidateModelAliases(out.ModelAliases); err != nil {
		return ColonyProfile{}, fmt.Errorf("colony: profile %s: %w", path, err)
	}
	beeAllowed := map[string]struct{}{"adapter": {}, "params": {}}
	for role, entry := range raw.Bees {
		if err := validateRole(role); err != nil {
			return ColonyProfile{}, fmt.Errorf("colony: profile %s: %w", path, err)
		}
		for k := range entry {
			if _, ok := beeAllowed[k]; !ok {
				return ColonyProfile{}, fmt.Errorf("colony: profile %s: bees.%s: forbidden key %q", path, role, k)
			}
		}
		overlay := BeeProfileOverlay{}
		if v, ok := entry["adapter"]; ok && v != nil {
			s, ok := v.(string)
			if !ok {
				return ColonyProfile{}, fmt.Errorf("colony: profile %s: bees.%s.adapter must be a string", path, role)
			}
			if err := validateProfileAdapter(s); err != nil {
				return ColonyProfile{}, fmt.Errorf("colony: profile %s: bees.%s: %w", path, role, err)
			}
			overlay.Adapter = strings.TrimSpace(s)
		}
		if v, ok := entry["params"]; ok && v != nil {
			params, ok := v.(map[string]any)
			if !ok {
				return ColonyProfile{}, fmt.Errorf("colony: profile %s: bees.%s.params must be a map", path, role)
			}
			overlay.Params = params
		}
		out.Bees[role] = overlay
	}
	return out, nil
}

func loadHomeProfileConfig(path string) (HomeProfileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return HomeProfileConfig{}, fmt.Errorf("colony: read profile %s: %w", path, err)
	}
	keys, err := yamlTopLevelKeys(data)
	if err != nil {
		return HomeProfileConfig{}, fmt.Errorf("colony: parse profile %s: %w", path, err)
	}
	allowed := map[string]struct{}{"nats": {}, "model_aliases": {}}
	for _, k := range keys {
		if _, ok := allowed[k]; !ok {
			return HomeProfileConfig{}, fmt.Errorf("colony: home profile %s: forbidden key %q", path, k)
		}
	}
	var raw HomeProfileConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return HomeProfileConfig{}, fmt.Errorf("colony: parse profile %s: %w", path, err)
	}
	raw.ModelAliases = NormalizeModelAliases(raw.ModelAliases)
	if err := ValidateModelAliases(raw.ModelAliases); err != nil {
		return HomeProfileConfig{}, fmt.Errorf("colony: home profile %s: %w", path, err)
	}
	return raw, nil
}

func yamlTopLevelKeys(data []byte) ([]string, error) {
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if len(m) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys, nil
}

func rejectColonyManifestProfileKey(data []byte, path string) error {
	keys, err := yamlTopLevelKeys(data)
	if err != nil {
		return nil // parse error handled by caller
	}
	for _, k := range keys {
		if k == "profile" {
			return fmt.Errorf("colony: %s: profile sticky belongs in home config.yaml, not colony.yaml", path)
		}
	}
	return nil
}
