// SPDX-FileCopyrightText: 2024 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/upjet/v2/pkg/controller"

	brandingconfiguration "github.com/ThePhaseless/terraform-provider-jellyfin/crossplane/internal/controller/namespaced/jellyfin/brandingconfiguration"
	library "github.com/ThePhaseless/terraform-provider-jellyfin/crossplane/internal/controller/namespaced/jellyfin/library"
	networkingconfiguration "github.com/ThePhaseless/terraform-provider-jellyfin/crossplane/internal/controller/namespaced/jellyfin/networkingconfiguration"
	plugin "github.com/ThePhaseless/terraform-provider-jellyfin/crossplane/internal/controller/namespaced/jellyfin/plugin"
	pluginconfiguration "github.com/ThePhaseless/terraform-provider-jellyfin/crossplane/internal/controller/namespaced/jellyfin/pluginconfiguration"
	pluginrepository "github.com/ThePhaseless/terraform-provider-jellyfin/crossplane/internal/controller/namespaced/jellyfin/pluginrepository"
	scheduledtask "github.com/ThePhaseless/terraform-provider-jellyfin/crossplane/internal/controller/namespaced/jellyfin/scheduledtask"
	systemconfiguration "github.com/ThePhaseless/terraform-provider-jellyfin/crossplane/internal/controller/namespaced/jellyfin/systemconfiguration"
)

// Setup creates all controllers with the supplied logger and adds them to
// the supplied manager.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		brandingconfiguration.Setup,
		library.Setup,
		networkingconfiguration.Setup,
		plugin.Setup,
		pluginconfiguration.Setup,
		pluginrepository.Setup,
		scheduledtask.Setup,
		systemconfiguration.Setup,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}

// SetupGated creates all controllers with the supplied logger and adds them to
// the supplied manager gated.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		brandingconfiguration.SetupGated,
		library.SetupGated,
		networkingconfiguration.SetupGated,
		plugin.SetupGated,
		pluginconfiguration.SetupGated,
		pluginrepository.SetupGated,
		scheduledtask.SetupGated,
		systemconfiguration.SetupGated,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}

// SetupWebhookWithManager registers conversion webhooks for all resource kinds in the group.
func SetupWebhookWithManager(mgr ctrl.Manager) error {
	for _, setup := range []func(ctrl.Manager) error{
		brandingconfiguration.SetupWebhookWithManager,
		library.SetupWebhookWithManager,
		networkingconfiguration.SetupWebhookWithManager,
		plugin.SetupWebhookWithManager,
		pluginconfiguration.SetupWebhookWithManager,
		pluginrepository.SetupWebhookWithManager,
		scheduledtask.SetupWebhookWithManager,
		systemconfiguration.SetupWebhookWithManager,
	} {
		if err := setup(mgr); err != nil {
			return err
		}
	}
	return nil
}
