package showprep

type Availability string

const (
	Ready          Availability = "ready"
	NotReceived    Availability = "not_received"
	Sold           Availability = "sold"
	Refunded       Availability = "refunded"
	CampaignClosed Availability = "campaign_closed"
	Removed        Availability = "removed"
	Unknown        Availability = "unknown"
)

func AvailabilityOf(p Purchase) Availability {
	switch {
	case !p.Known:
		return Unknown
	case !p.Exists || !p.CampaignExists:
		return Removed
	case p.Sold:
		return Sold
	case p.Refunded:
		return Refunded
	case p.Phase == "closed":
		return CampaignClosed
	case p.Phase != "active" && p.Phase != "pending":
		return Unknown
	case !p.Received:
		return NotReceived
	default:
		return Ready
	}
}
func (a Availability) CanAdd() bool  { return a == Ready || a == NotReceived }
func (a Availability) CanPack() bool { return a == Ready }
