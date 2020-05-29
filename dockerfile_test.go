package dockerfileyml

import (
	"bytes"
	"testing"

	. "github.com/go-courier/snapshotmacther"
	. "github.com/onsi/gomega"
)

func TestDockerfile(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		key := "key"

		d := Dockerfile{}
		d.From = "busybox:latest"
		d.WorkingDir = "/todo"

		d.Env = Values{
			key: "hello",
		}

		d.Copy = Values{
			"x": "./",
		}

		d.Entrypoint = Args("sh")
		d.Command = Args("-c", "echo", ContainerEnvVar(key))

		buf := bytes.NewBuffer(nil)
		err := WriteToDockerfile(buf, d)
		NewWithT(t).Expect(err).To(BeNil())
		NewWithT(t).Expect(buf.String()).To(MatchSnapshot("simple.Dockerfile"))
	})

	t.Run("multi stages", func(t *testing.T) {
		d := Dockerfile{}

		d.Stages = map[string]*Stage{
			"builder": {
				From:       "busybox",
				WorkingDir: "/go/src",
				Run:        Scripts("touch a.txt", "touch b.txt"),
			},
			"builder2": {
				From:       "busybox",
				WorkingDir: "/go/src",
				Run: Scripts(
					"touch b.txt",
				),
			},
		}

		d.From = "busybox"
		d.WorkingDir = "/todo"
		d.Copy = Values{
			"builder:./a.txt":  "./",
			"builder2:./b.txt": "./",
		}

		buf := bytes.NewBuffer(nil)
		err := WriteToDockerfile(buf, d)
		NewWithT(t).Expect(err).To(BeNil())
		NewWithT(t).Expect(buf.String()).To(MatchSnapshot("multistage.Dockerfile"))
	})
}
