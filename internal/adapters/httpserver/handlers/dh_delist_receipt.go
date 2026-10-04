package handlers

import "github.com/guarzo/slabledger/internal/adapters/clients/dh"

// A whole old-target delist must acknowledge that target, with no remaining
// live/error channels. Merely receiving a nil error is not completion evidence.
func validFullDelistReceipt(receipt *dh.ChannelSyncResponse, target int) bool {
	if receipt == nil || receipt.DHInventoryID != target || (receipt.Status != "in_stock" && receipt.Status != "listed") {
		return false
	}
	for _, channel := range receipt.Channels {
		if channel.Status != "removed" && channel.Status != "delisted" {
			return false
		}
	}
	return true
}
