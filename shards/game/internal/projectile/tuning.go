package projectile

import "fmt"

// ProjectileTuning is data-driven shot config, loaded once at boot from content/projectile/tuning.json.
// Read it via data.Get[ProjectileTuning]().
type ProjectileTuning struct {
	Launchers map[string]Launcher `json:"launchers"` // by the ability ID that launches the shot
}

// Launcher is the shot an ability launches.
type Launcher struct {
	Speed  float64 `json:"speed"`  // world units per second
	Range  float64 `json:"range"`  // world units flown before it expires
	Radius float64 `json:"radius"` // world units
	Damage float64 `json:"damage"` // to an enemy Echo
}

// Name is the data-catalog key used by data.Get[ProjectileTuning]().
func (ProjectileTuning) Name() string { return "projectile_tuning" }

// JSONFile is the path within the content embed.FS.
func (ProjectileTuning) JSONFile() string { return "projectile/tuning.json" }

// Validate runs at load time; a returned error panics the boot — fail loud on bad content.
func (t ProjectileTuning) Validate() error {
	for id, l := range t.Launchers {
		if l.Speed <= 0 || l.Range <= 0 || l.Radius <= 0 || l.Damage < 0 {
			return fmt.Errorf("projectile_tuning: %q needs speed, range, radius > 0 and damage >= 0, got %+v", id, l)
		}
	}
	return nil
}
