package bake_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"echo_image/pkg/level"
	"echo_image/pkg/level/bake"
)

// The project holds a class for each kind but a fill, which nobody draws, and each object type, the
// type's members its props' fields with their zero values, as Tiled's project file has them.
func TestProjectIsTheSchema(t *testing.T) {
	data, err := bake.Project(config)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "testdata/yard.tiled-project", data)
	var project struct {
		PropertyTypes []struct {
			Name    string           `json:"name"`
			Type    string           `json:"type"`
			UseAs   []string         `json:"useAs"`
			Members []map[string]any `json:"members"`
		} `json:"propertyTypes"`
	}
	if err := json.Unmarshal(data, &project); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range project.PropertyTypes {
		if c.Type != "class" || !reflect.DeepEqual(c.UseAs, []string{"object"}) {
			t.Errorf("%s is a %s used as %v, want an object class", c.Name, c.Type, c.UseAs)
		}
		got = append(got, c.Name)
	}
	if want := []string{"bounds", "wall", "spawn"}; !reflect.DeepEqual(got, want) {
		t.Errorf("classes %v, want the kinds then the types, each sorted: %v", got, want)
	}
	if members := project.PropertyTypes[2].Members; !reflect.DeepEqual(members,
		[]map[string]any{{"name": "team", "type": "int", "value": 0.0}}) {
		t.Errorf("a spawn's members %v, want team, an int of 0", members)
	}
}

// A props field Tiled can't hold has no project.
func TestProjectRefusesAFieldTiledCantHold(t *testing.T) {
	type squad struct {
		Members []string `json:"members"`
	}
	c := config
	c.Schema.Types = level.Types{"squad": reflect.TypeFor[squad]()}
	if _, err := bake.Project(c); err == nil || !strings.Contains(err.Error(), "field Members is a []string") {
		t.Errorf("got %v, want an error naming the field", err)
	}
}
