package dockerfileyml

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

type Dockerfile struct {
	Image  string            `yaml:"image,omitempty"`
	Stages map[string]*Stage `yaml:"stages,omitempty"`
	Stage  `yaml:",inline"`
}

func Scripts(args ...string) []string {
	return args
}

func Args(args ...string) []string {
	return args
}

type Values = map[string]string

type Stage struct {
	From       string            `yaml:"from,omitempty" docker:"FROM" `
	Label      map[string]string `yaml:"label,omitempty" docker:"LABEL,multi" `
	WorkingDir string            `yaml:"workdir" docker:"WORKDIR" `

	Env  Values   `yaml:"env,omitempty" docker:"ENV,multi"`
	Add  Values   `yaml:"add,omitempty" docker:"ADD,join"`
	Copy Values   `yaml:"copy,omitempty" docker:"COPY"`
	Run  []string `yaml:"run,omitempty" docker:"RUN,script"`

	Expose []string `yaml:"expose,omitempty" docker:"EXPOSE"`
	Volume []string `yaml:"volume,omitempty" docker:"VOLUME,array"`

	Entrypoint []string `yaml:"entrypoint,omitempty" docker:"ENTRYPOINT,array"`
	Command    []string `yaml:"cmd,omitempty" docker:"CMD,array"`

	usedBy       map[string]bool
	name         string
	copyReplaces map[string]string
}

func scanAndValidate(s *Stage, stages map[string]*Stage) error {
	for from := range s.Copy {
		parts := strings.Split(from, ":")

		if len(parts) == 2 {
			stageName := parts[0]

			if stage, ok := stages[stageName]; ok {
				if stage.WorkingDir == "" {
					return fmt.Errorf("stage %s must define workdir for copy file", stageName)
				}

				if stage.usedBy == nil {
					stage.usedBy = map[string]bool{}
				}

				stage.usedBy[s.name] = true

				if s.copyReplaces == nil {
					s.copyReplaces = map[string]string{}
				}

				s.copyReplaces[from] = "--from=" + stageName + " " + filepath.Join(stage.WorkingDir, parts[1])
			} else {
				return fmt.Errorf("missing stage %s", stageName)
			}
		}
	}
	return nil
}

func WriteToDockerfile(w io.Writer, d Dockerfile) error {
	stages := make([]*Stage, 0)

	for name := range d.Stages {
		s := d.Stages[name]
		s.name = name

		if err := scanAndValidate(s, d.Stages); err != nil {
			return err
		}

		stages = append(stages, s)
	}

	if err := scanAndValidate(&d.Stage, d.Stages); err != nil {
		return err
	}

	sort.Slice(stages, func(i, j int) bool {
		return len(stages[i].usedBy) > len(stages[j].usedBy) || stages[i].name < stages[j].name
	})

	for i := range stages {
		if err := writeState(w, stages[i]); err != nil {
			return err
		}
	}

	if err := writeState(w, &d.Stage); err != nil {
		return err
	}

	return nil
}

func writeState(w io.Writer, stage *Stage) error {
	if stage == nil {
		return nil
	}

	write := func(dockerKey string, values ...string) {
		if len(values) == 0 {
			return
		}

		_, _ = io.WriteString(w, dockerKey)

		for i := range values {
			_, _ = io.WriteString(w, " ")

			v := values[i]

			switch dockerKey {
			case "FROM":
				if stage.name != "" {
					v += " as " + stage.name
				}
			case "COPY":
				if stage.copyReplaces != nil {
					if replaced, ok := stage.copyReplaces[v]; ok {
						v = replaced
					}
				}
			}

			_, _ = io.WriteString(w, v)
		}

		_, _ = io.WriteString(w, "\n")
	}

	rv := reflect.Indirect(reflect.ValueOf(stage))
	tpe := rv.Type()

	for i := 0; i < tpe.NumField(); i++ {
		field := tpe.Field(i)
		dockerTag := field.Tag.Get("docker")
		dockerKeys := strings.Split(dockerTag, ",")
		dockerKey := dockerKeys[0]
		dockerFlags := dockerKeys[1:]

		if len(dockerKey) > 0 {
			value := rv.FieldByName(field.Name)

			switch field.Type.Kind() {
			case reflect.String:
				if len(value.String()) > 0 {
					inline := stringIncludes(dockerFlags, "inline")
					if inline {
						write(dockerKey, value.String())
					} else {
						write(dockerKey, mayQuote(value.String()))
					}
				}
			case reflect.Slice:
				jsonArray := stringIncludes(dockerFlags, "array")
				slice := make([]string, 0)

				for i := 0; i < value.Len(); i++ {
					slice = append(slice, value.Index(i).String())
				}

				if len(slice) > 0 {
					if jsonArray {
						jsonString, err := json.Marshal(slice)
						if err != nil {
							panic(err)
						}
						write(
							dockerKey,
							string(jsonString),
						)
					} else {
						if stringIncludes(dockerFlags, "script") {
							write(
								dockerKey,
								strings.Join(slice, " && "),
							)
						} else {
							write(
								dockerKey,
								strings.Join(slice, ""),
							)
						}
					}
				}

			case reflect.Map:
				multi := stringIncludes(dockerFlags, "multi")
				join := stringIncludes(dockerFlags, "join")

				if join {
					destMap := map[string][]string{}

					for _, key := range value.MapKeys() {
						dest := value.MapIndex(key).String()
						k := key.String()

						if destMap[dest] == nil {
							destMap[dest] = []string{}
						}
						destMap[dest] = append(destMap[dest], k)
					}

					for dest, src := range destMap {
						sort.Strings(src)

						write(
							dockerKey,
							append(src, dest)...,
						)
					}
				} else {
					keys := make([]string, 0)
					values := map[string]string{}

					for _, key := range value.MapKeys() {
						k := key.String()
						keys = append(keys, k)
						values[k] = value.MapIndex(key).String()
					}

					sort.Strings(keys)

					if multi {
						keyValues := make([]string, 0)

						for _, key := range keys {
							keyValues = append(keyValues, key+"="+mayQuote(values[key]))
						}

						if len(keyValues) > 0 {
							write(dockerKey, keyValues...)
						}
					} else {
						for _, key := range keys {
							write(dockerKey, key, mayQuote(values[key]))
						}
					}
				}
			}
		}
	}

	return nil
}

func mayQuote(s string) string {
	if s == "" || strings.Contains(s, " ") {
		return strconv.Quote(s)
	}
	return s
}

func stringIncludes(list []string, target string) bool {
	return stringSome(list, func(item string, i int) bool {
		return item == target
	})
}

func stringSome(list []string, checker func(item string, i int) bool) bool {
	for i, item := range list {
		if checker(item, i) {
			return true
		}
	}
	return false
}
