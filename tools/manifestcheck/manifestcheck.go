package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bazelbuild/rules_go/go/runfiles"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsvalidation "k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/teapotovh/teapot/tools/util"
)

const (
	crdKind           = "CustomResourceDefinition"
	decoderBufferSize = 4096
	deployDirectory   = "deploy"
	directoryMode     = 0o700
	fileMode          = 0o600
)

var (
	verbose = flag.Bool("verbose", false, "print all executed commands")
	strict  = flag.Bool("strict", true, "reject fields that are not in the schema")
)

var (
	fluxKustomizationGroup   = "kustomize.toolkit.fluxcd.io"
	fluxKustomizationKind    = "Kustomization"
	fluxKustomizationVersion = fluxKustomizationGroup + "/v1"
)

var baseDirectories = []string{
	"deploy/base",
	"deploy/base/flux",
	"deploy/base/gitops",
}

type kustomization struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Spec       struct {
		Path string `json:"path"`
	} `json:"spec"`
}

func main() {
	flag.Parse()

	if err := check(); err != nil {
		fmt.Fprintln(os.Stderr, "manifestcheck: "+err.Error())
		os.Exit(1)
	}
}

func check() (err error) {
	kustomizePath, err := runfiles.Rlocation("multitool/tools/kustomize/kustomize")
	if err != nil {
		return fmt.Errorf("could not fetch kustomize path: %w", err)
	}

	kubeconformPath, err := runfiles.Rlocation("multitool/tools/kubeconform/kubeconform")
	if err != nil {
		return fmt.Errorf("could not fetch kubeconform path: %w", err)
	}

	yamlfmtPath, err := runfiles.Rlocation("multitool/tools/yamlfmt/yamlfmt")
	if err != nil {
		return fmt.Errorf("could not fetch yamlfmt path: %w", err)
	}

	util.SetVerbose(*verbose)

	root, err := util.WorkspaceRoot()
	if err != nil {
		return err
	}

	util.SetRoot(root)

	if err := checkFormatting(yamlfmtPath); err != nil {
		return err
	}

	entrypoints, err := entrypoints(filepath.Join(root, deployDirectory))
	if err != nil {
		return err
	}

	temp, err := os.MkdirTemp("", "teapot-manifests-")
	if err != nil {
		return fmt.Errorf("creating temporary directory: %w", err)
	}

	defer func() {
		err = errors.Join(err, os.RemoveAll(temp))
	}()

	manifestDir := filepath.Join(temp, "manifests")
	schemaDir := filepath.Join(temp, "schemas")

	if err := os.MkdirAll(manifestDir, directoryMode); err != nil {
		return fmt.Errorf("creating manifest directory: %w", err)
	}

	if err := os.MkdirAll(schemaDir, directoryMode); err != nil {
		return fmt.Errorf("creating schema directory: %w", err)
	}

	for _, dir := range entrypoints {
		util.Phase("(check) kustomize build " + dir)

		stdout, stderr, err := util.Run(nil, kustomizePath, "build", dir)
		if err != nil {
			return fmt.Errorf("kustomize build %s failed: %w, %s", dir, err, stderr.String())
		}

		name := strings.ReplaceAll(dir, string(filepath.Separator), "_") + ".yaml"
		if err := os.WriteFile(filepath.Join(manifestDir, name), stdout.Bytes(), fileMode); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}

	util.Phase("(check) CRD schemas")

	if err := writeCRDSchemas(manifestDir, schemaDir); err != nil {
		return err
	}

	return runKubeconform(kubeconformPath, schemaDir, manifestDir)
}

func checkFormatting(yamlfmtPath string) error {
	util.Phase("(check) yamlfmt")

	stdout, stderr, err := util.Run(nil, yamlfmtPath, "-lint", deployDirectory)
	fmt.Fprint(os.Stderr, stdout.String())
	fmt.Fprint(os.Stderr, stderr.String())

	if err != nil {
		return fmt.Errorf("yamlfmt failed: %w", err)
	}

	return nil
}

func runKubeconform(kubeconformPath, schemaDir, manifestDir string) error {
	util.Phase("(check) kubeconform")

	args := []string{"-summary", "-skip", crdKind}
	if *strict {
		args = append(args, "-strict")
	}

	args = append(
		args,
		"-schema-location",
		"default",
		"-schema-location",
		filepath.ToSlash(filepath.Join(schemaDir, "{{.Group}}_{{.ResourceKind}}_{{.ResourceAPIVersion}}.json")),
		manifestDir,
	)

	stdout, stderr, err := util.Run(nil, kubeconformPath, args...)
	fmt.Fprint(os.Stderr, stdout.String())
	fmt.Fprint(os.Stderr, stderr.String())

	if err != nil {
		return fmt.Errorf("kubeconform failed: %w", err)
	}

	return nil
}

func isYAML(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))

	return ext == ".yaml" || ext == ".yml"
}

// entrypoints returns the base directories and every directory referenced by
// a Flux Kustomization. These are the manifests Flux applies to the cluster.
func entrypoints(deploy string) ([]string, error) {
	set := make(map[string]struct{}, len(baseDirectories))

	for _, dir := range baseDirectories {
		set[dir] = struct{}{}
	}

	err := filepath.WalkDir(deploy, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			return nil
		}

		if !isYAML(path) {
			return nil
		}

		return addFluxPaths(path, set)
	})
	if err != nil {
		return nil, err
	}

	paths := make([]string, 0, len(set))
	for path := range set {
		paths = append(paths, path)
	}

	sort.Strings(paths)

	return paths, nil
}

// addFluxPaths adds every path referenced by a Flux Kustomization in a file.
func addFluxPaths(path string, set map[string]struct{}) (err error) {
	file, err := os.Open(path) //nolint:gosec
	if err != nil {
		return err
	}

	defer func() {
		err = errors.Join(err, file.Close())
	}()

	decoder := yaml.NewYAMLOrJSONDecoder(file, decoderBufferSize)

	for {
		var document kustomization

		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return fmt.Errorf("could not parse %s: %w", path, err)
		}

		if document.APIVersion == fluxKustomizationVersion &&
			document.Kind == fluxKustomizationKind &&
			document.Spec.Path != "" {
			set[document.Spec.Path] = struct{}{}
		}
	}
}

// writeCRDSchemas writes JSON schemas for every CustomResourceDefinition in
// the built manifests. The CRDs are pinned in the repository, so the schemas
// match the versions that the cluster applies.
func writeCRDSchemas(manifestDir, schemaDir string) error {
	err := filepath.WalkDir(manifestDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			return nil
		}

		if !isYAML(path) {
			return nil
		}

		return writeCRDSchemasFromFile(path, schemaDir)
	})
	if err != nil {
		return fmt.Errorf("writing CRD schemas: %w", err)
	}

	return nil
}

func writeCRDSchemasFromFile(path, schemaDir string) (err error) {
	file, err := os.Open(path) //nolint:gosec
	if err != nil {
		return err
	}

	defer func() {
		err = errors.Join(err, file.Close())
	}()

	decoder := yaml.NewYAMLOrJSONDecoder(file, decoderBufferSize)

	for {
		var crd apiextensionsv1.CustomResourceDefinition

		err := decoder.Decode(&crd)
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return fmt.Errorf("could not parse %s: %w", path, err)
		}

		if crd.Kind != crdKind {
			continue
		}

		if err := writeCRDSchema(&crd, schemaDir); err != nil {
			return fmt.Errorf("writing schema from %s: %w", path, err)
		}
	}
}

func writeCRDSchema(crd *apiextensionsv1.CustomResourceDefinition, schemaDir string) error {
	for _, version := range crd.Spec.Versions {
		if version.Schema == nil || version.Schema.OpenAPIV3Schema == nil {
			continue
		}

		var internal apiextensions.JSONSchemaProps
		if err := apiextensionsv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(
			version.Schema.OpenAPIV3Schema,
			&internal,
			nil,
		); err != nil {
			return fmt.Errorf("converting schema for %s/%s: %w", crd.Spec.Group, crd.Spec.Names.Kind, err)
		}

		var schema spec.Schema
		if err := apiextensionsvalidation.ConvertJSONSchemaProps(&internal, &schema); err != nil {
			return fmt.Errorf("converting schema for %s/%s: %w", crd.Spec.Group, crd.Spec.Names.Kind, err)
		}

		data, err := json.Marshal(schema)
		if err != nil {
			return fmt.Errorf("marshalling schema for %s/%s: %w", crd.Spec.Group, crd.Spec.Names.Kind, err)
		}

		name := fmt.Sprintf("%s_%s_%s.json", crd.Spec.Group, strings.ToLower(crd.Spec.Names.Kind), version.Name)
		if err := os.WriteFile(filepath.Join(schemaDir, name), append(data, '\n'), fileMode); err != nil {
			return fmt.Errorf("writing schema for %s/%s: %w", crd.Spec.Group, crd.Spec.Names.Kind, err)
		}
	}

	return nil
}
