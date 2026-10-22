// SPDX-FileCopyrightText: 2026 Sofie Finnes <sofie+git@finnes.dev>
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	api "github.com/ThePhaseless/terraform-provider-jellyfin/crossplane/apis/namespaced/jellyfin/v1alpha1"
	configapi "github.com/ThePhaseless/terraform-provider-jellyfin/crossplane/apis/namespaced/v1beta1"
	jellyfin "github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	xpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/crossplane/upjet/v2/pkg/terraform"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type sessionFixture struct {
	mu          sync.Mutex
	initialized bool
	writes      int
	logins      int
	password    string
	sessions    map[string]string
}

func (f *sessionFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/System/Info/Public":
		_ = json.NewEncoder(w).Encode(map[string]any{"StartupWizardCompleted": f.initialized, "Version": "12.2"})
	case "/Startup/Configuration", "/Startup/User", "/Startup/Complete":
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"Name":"","Password":""}`))
			return
		}
		f.writes++
		if r.URL.Path == "/Startup/Complete" {
			f.initialized = true
		}
		w.WriteHeader(http.StatusNoContent)
	case "/Users/AuthenticateByName":
		var credentials struct{ Username, Pw string }
		if json.NewDecoder(r.Body).Decode(&credentials) != nil || credentials.Username != "admin" || credentials.Pw != f.password {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		match := regexp.MustCompile(`DeviceId="([^"]+)"`).FindStringSubmatch(r.Header.Get("Authorization"))
		if len(match) != 2 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.logins++
		token := fmt.Sprintf("token-%d", f.logins)
		f.sessions[match[1]] = token
		_ = json.NewEncoder(w).Encode(map[string]string{"AccessToken": token})
	case "/Users":
		token := strings.TrimSuffix(strings.TrimPrefix(r.Header.Get("Authorization"), `MediaBrowser Token="`), `"`)
		for _, current := range f.sessions {
			if token == current {
				_, _ = w.Write([]byte(`[]`))
				return
			}
		}
		w.WriteHeader(http.StatusUnauthorized)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func sessionKube(t *testing.T, endpoint string) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = configapi.AddToScheme(scheme)
	pc := &configapi.ProviderConfig{ObjectMeta: metav1.ObjectMeta{Name: "jellyfin", Namespace: "media"}, Spec: configapi.ProviderConfigSpec{
		Endpoint: endpoint, Username: "admin", PasswordSecretRef: configapi.CredentialsReference{Name: "credentials", Key: "password"},
	}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "media"}, Data: map[string][]byte{"password": []byte("old-password")}}
	return clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(pc, secret).Build()
}

func configureSession(t *testing.T, kube client.Client, resource xpresource.Managed) provider.ConfigureResponse {
	t.Helper()
	configured, err := setup(t.Context(), kube, resource)
	if err != nil {
		t.Fatal(err)
	}
	return configureFramework(t, configured)
}

func configureFramework(t *testing.T, configured terraform.Setup) provider.ConfigureResponse {
	t.Helper()
	var schema provider.SchemaResponse
	configured.FrameworkProvider.Schema(t.Context(), provider.SchemaRequest{}, &schema)
	if schema.Diagnostics.HasError() {
		t.Fatal(schema.Diagnostics)
	}
	values := map[string]tftypes.Value{}
	for name := range schema.Schema.Attributes {
		values[name] = tftypes.NewValue(tftypes.String, configured.Configuration[name])
	}
	req := provider.ConfigureRequest{Config: tfsdk.Config{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(schema.Schema.Type().TerraformType(t.Context()), values),
	}}
	var response provider.ConfigureResponse
	configured.FrameworkProvider.Configure(t.Context(), req, &response)
	return response
}

func sessionResources() []xpresource.ModernManaged {
	objects := []xpresource.ModernManaged{&api.Library{}, &api.BrandingConfiguration{}}
	for i, obj := range objects {
		obj.SetNamespace("media")
		obj.SetName(fmt.Sprintf("resource-%d", i))
		obj.SetUID(types.UID(fmt.Sprintf("uid-%d", i)))
		obj.SetProviderConfigReference(&xpv2.ProviderConfigReference{Kind: "ProviderConfig", Name: "jellyfin"})
	}
	return objects
}

func TestBootstrapRequiresCreate(t *testing.T) {
	for _, policies := range []xpv2.ManagementPolicies{
		{}, {xpv2.ManagementActionObserve}, {xpv2.ManagementActionUpdate}, {xpv2.ManagementActionDelete},
		{xpv2.ManagementActionCreate}, {xpv2.ManagementActionAll},
	} {
		t.Run(fmt.Sprint(policies), func(t *testing.T) {
			for _, deleting := range []bool{false, true} {
				f := &sessionFixture{password: "old-password", sessions: map[string]string{}}
				server := httptest.NewServer(f)
				defer server.Close()
				obj := sessionResources()[0]
				obj.SetManagementPolicies(policies)
				if deleting {
					now := metav1.Now()
					obj.SetDeletionTimestamp(&now)
				}
				response := configureSession(t, sessionKube(t, server.URL), obj)
				permitted := !deleting && len(policies) == 1 && (policies[0] == xpv2.ManagementActionCreate || policies[0] == xpv2.ManagementActionAll)
				if permitted {
					if response.Diagnostics.HasError() || !f.initialized || f.writes != 3 {
						t.Fatalf("bootstrap failed: writes=%d, diagnostics=%v", f.writes, response.Diagnostics)
					}
				} else if !response.Diagnostics.HasError() || f.writes != 0 || f.logins != 0 {
					t.Fatalf("unauthorized bootstrap: deleting=%v, writes=%d, logins=%d, diagnostics=%v", deleting, f.writes, f.logins, response.Diagnostics)
				}
			}
		})
	}
}

func TestConcurrentResourceSessions(t *testing.T) {
	for _, initialized := range []bool{false, true} {
		t.Run(fmt.Sprintf("initialized=%v", initialized), func(t *testing.T) {
			f := &sessionFixture{initialized: initialized, password: "old-password", sessions: map[string]string{}}
			server := httptest.NewServer(f)
			defer server.Close()
			kube := sessionKube(t, server.URL)
			objects := sessionResources()
			responses := make([]provider.ConfigureResponse, len(objects))
			var wg sync.WaitGroup
			for i, obj := range objects {
				obj.SetManagementPolicies(xpv2.ManagementPolicies{xpv2.ManagementActionAll})
				wg.Go(func() { responses[i] = configureSession(t, kube, obj) })
			}
			wg.Wait()
			for _, response := range responses {
				if response.Diagnostics.HasError() {
					t.Fatal(response.Diagnostics)
				}
				c := response.ResourceData.(*jellyfin.Client)
				defer c.HTTPClient.CloseIdleConnections()
				if _, err := c.GetUsers(t.Context()); err != nil {
					t.Fatalf("another controller invalidated the session: %v", err)
				}
			}
			expectedWrites := 0
			if !initialized {
				expectedWrites = 3
			}
			if f.writes != expectedWrites {
				t.Fatalf("startup writes=%d, want %d", f.writes, expectedWrites)
			}
			f.mu.Lock()
			f.password = "rotated-password"
			f.mu.Unlock()
			secret := &corev1.Secret{}
			if err := kube.Get(t.Context(), types.NamespacedName{Namespace: "media", Name: "credentials"}, secret); err != nil {
				t.Fatal(err)
			}
			secret.Data["password"] = []byte("rotated-password")
			if err := kube.Update(t.Context(), secret); err != nil {
				t.Fatal(err)
			}
			response := configureSession(t, kube, objects[0])
			if response.Diagnostics.HasError() {
				t.Fatalf("rotated credentials failed: %v", response.Diagnostics)
			}
			c := response.ResourceData.(*jellyfin.Client)
			defer c.HTTPClient.CloseIdleConnections()
			if _, err := c.GetUsers(t.Context()); err != nil {
				t.Fatal(err)
			}
			if _, err := responses[1].ResourceData.(*jellyfin.Client).GetUsers(t.Context()); err != nil {
				t.Fatalf("credential rotation invalidated another resource: %v", err)
			}
		})
	}
}
