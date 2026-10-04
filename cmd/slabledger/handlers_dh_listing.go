package main

import (
	"context"

	dhadapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

func buildHTTPListingService(ctx context.Context, in handlerInputs) dhlisting.Service {
	if in.DHClient == nil {
		return nil
	}
	opts := []dhlisting.DHListingServiceOption{
		dhlisting.WithDHListingMutationCoordinator(in.MutationCoordinator, in.ReturnStore, in.ReturnStore),
		dhlisting.WithDHListingLister(dhadapter.NewInventoryAdapter(in.DHClient).WithMutationReceipts()),
		dhlisting.WithDHListingPSAImporter(dhadapter.NewPSAImporterAdapter(in.DHClient)),
	}
	if in.PurchaseStore != nil {
		opts = append(opts, dhlisting.WithDHListingFieldsUpdater(in.PurchaseStore), dhlisting.WithDHListingPushStatusUpdater(in.PurchaseStore), dhlisting.WithDHListingResetter(in.PurchaseStore), dhlisting.WithDHListingUnlistedClearer(in.PurchaseStore))
	}
	if in.CardIDMappingRepo != nil {
		opts = append(opts, dhlisting.WithDHListingCardIDSaver(in.CardIDMappingRepo))
	}
	if in.DHEventStore != nil {
		opts = append(opts, dhlisting.WithEventRecorder(in.DHEventStore))
	}
	if in.DHStore != nil {
		opts = append(opts, dhlisting.WithDHListingConfigLoader(in.DHStore))
	}
	svc, e := dhlisting.NewDHListingService(in.CampaignsService, in.Logger, opts...)
	if e != nil {
		in.Logger.Error(ctx, "create DH listing service", observability.Err(e))
		return nil
	}
	return svc
}
