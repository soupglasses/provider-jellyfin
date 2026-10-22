// SPDX-FileCopyrightText: 2026 Sofie Finnes <sofie+git@finnes.dev>
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	jellyfin "github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
	upstream "github.com/ThePhaseless/terraform-provider-jellyfin/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Bootstrap spans several requests and must finish before another kind attempts it.
var bootstrapMu sync.Mutex

type resourceProvider struct {
	provider.Provider
	deviceID       string
	allowBootstrap bool
}

func (p *resourceProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data upstream.JellyfinProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	c := jellyfin.NewClient(data.Endpoint.ValueString(), "")
	defer c.HTTPClient.CloseIdleConnections()
	info, err := c.GetPublicSystemInfo(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Cannot inspect Jellyfin startup", err.Error())
		return
	}
	if !info.StartupWizardCompleted {
		if !p.allowBootstrap {
			resp.Diagnostics.AddError("Jellyfin setup requires Create", "Initial server setup is disabled unless this resource permits Create and is not being deleted.")
			return
		}
		if !p.bootstrap(ctx, req, resp, c) {
			return
		}
	}
	// Jellyfin replaces earlier sessions for the same device ID. Each resource
	// needs its own identity because different kind controllers run concurrently.
	c.HTTPClient.Transport = &deviceTransport{RoundTripper: c.HTTPClient.Transport, deviceID: p.deviceID}
	auth, err := c.AuthenticateByName(ctx, data.Username.ValueString(), data.Password.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Cannot authenticate with Jellyfin", err.Error())
		return
	}
	if auth.AccessToken == "" {
		resp.Diagnostics.AddError("Cannot authenticate with Jellyfin", "Jellyfin returned an empty access token.")
		return
	}
	// Retain upstream configuration checks and version warnings, using our token
	// so its fixed-device password login cannot invalidate another resource.
	values := map[string]tftypes.Value{}
	if err := req.Config.Raw.As(&values); err != nil {
		resp.Diagnostics.AddError("Cannot configure Jellyfin session", err.Error())
		return
	}
	values["api_key"] = tftypes.NewValue(tftypes.String, auth.AccessToken)
	req.Config.Raw = tftypes.NewValue(req.Config.Raw.Type(), values)
	p.Provider.Configure(ctx, req, resp)
}

func (p *resourceProvider) bootstrap(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse, c *jellyfin.Client) bool {
	bootstrapMu.Lock()
	defer bootstrapMu.Unlock()
	info, err := c.GetPublicSystemInfo(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Cannot inspect Jellyfin startup", err.Error())
		return false
	}
	if !info.StartupWizardCompleted {
		p.Provider.Configure(ctx, req, resp)
	}
	return !resp.Diagnostics.HasError()
}

type deviceTransport struct {
	http.RoundTripper
	deviceID string
}

func (t *deviceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header.Set("Authorization", strings.ReplaceAll(req.Header.Get("Authorization"),
		`DeviceId="terraform-provider-jellyfin"`, fmt.Sprintf(`DeviceId=%q`, t.deviceID)))
	return t.RoundTripper.RoundTrip(cloned)
}

func (t *deviceTransport) CloseIdleConnections() {
	if closer, ok := t.RoundTripper.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}
