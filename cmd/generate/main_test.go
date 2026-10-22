// SPDX-FileCopyrightText: 2026 Sofie Finnes <sofie+git@finnes.dev>
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/cel-go/cel"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

func TestPluginConfigurationSecretAdmission(t *testing.T) {
	source, err := os.ReadFile("../../apis/namespaced/jellyfin/v1alpha1/zz_pluginconfiguration_types.go")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "types.go")
	// Reproduce Upjet's missing initProvider alternative before applying the correction.
	unpatched := strings.ReplaceAll(string(source), ` || (has(self.initProvider) && has(self.initProvider.configurationJsonSecretRef))`, "")
	if err := os.WriteFile(path, []byte(unpatched), 0600); err != nil {
		t.Fatal(err)
	}
	if err := allowInitialSecret(path); err != nil {
		t.Fatal(err)
	}
	if err := allowInitialSecret(path); err != nil {
		t.Fatal(err)
	}
	corrected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`XValidation:rule="([^"]+)"`).FindSubmatch(corrected)
	if len(match) != 2 {
		t.Fatal("missing Secret validation")
	}
	rule := string(match[1])
	raw, err := os.ReadFile("../../package/crds/jellyfin.finnes.dev_pluginconfigurations.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var crd apiextensionsv1.CustomResourceDefinition
	if err := yaml.Unmarshal(raw, &crd); err != nil {
		t.Fatal(err)
	}
	installedRule := crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].XValidations[0].Rule
	if rule != installedRule {
		t.Fatal("installed CRD does not match corrected generation")
	}
	env, err := cel.NewEnv(cel.Variable("self", cel.DynType))
	if err != nil {
		t.Fatal(err)
	}
	ast, issues := env.Compile(rule)
	if issues.Err() != nil {
		t.Fatal(issues.Err())
	}
	program, err := env.Program(ast)
	if err != nil {
		t.Fatal(err)
	}
	selector := map[string]any{"name": "settings", "key": "json"}
	for _, tc := range []struct {
		name                      string
		policies                  []string
		forProvider, initProvider map[string]any
		allowed                   bool
	}{
		{"forProvider", []string{"*"}, map[string]any{"configurationJsonSecretRef": selector}, nil, true},
		{"initProvider", []string{"*"}, map[string]any{}, map[string]any{"configurationJsonSecretRef": selector}, true},
		{"createFromInit", []string{"Observe", "Create"}, map[string]any{}, map[string]any{"configurationJsonSecretRef": selector}, true},
		{"missing", []string{"*"}, map[string]any{}, nil, false},
		{"emptyInit", []string{"Update"}, map[string]any{}, map[string]any{}, false},
		{"observe", []string{"Observe"}, map[string]any{}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := map[string]any{"managementPolicies": tc.policies, "forProvider": tc.forProvider}
			if tc.initProvider != nil {
				spec["initProvider"] = tc.initProvider
			}
			result, _, err := program.Eval(map[string]any{"self": spec})
			if err != nil {
				t.Fatal(err)
			}
			if result.Value() != tc.allowed {
				t.Fatalf("admitted=%v, want %v", result, tc.allowed)
			}
		})
	}
}
