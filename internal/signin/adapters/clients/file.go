package clients

import (
	"bytes"
	"errors"
	"io"
	"os"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// LoadFile puts the Client objects in a YAML file into x, for a local run or the e2e suite, where
// no cluster exists. The API server's rules (deploy/crd) are not applied to them.
func LoadFile(path string, x *Index) error {
	raw, err := os.ReadFile(path) //nolint:gosec // a path from the environment of a local run
	if err != nil {
		return err
	}
	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(raw), len(raw))
	for {
		var obj map[string]any
		err := dec.Decode(&obj)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		c, err := fromObject(&unstructured.Unstructured{Object: obj})
		if err != nil {
			return err
		}
		x.Put(c)
	}
}
