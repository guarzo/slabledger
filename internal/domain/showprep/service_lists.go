package showprep

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

func validName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 120 {
		return "", ErrInvalid
	}
	return name, nil
}
func (s *Service) Lists(ctx context.Context) ([]List, error) { return s.store.Lists(ctx) }
func (s *Service) CreateList(ctx context.Context, id, name string) (List, error) {
	name, err := validName(name)
	if err != nil {
		return List{}, err
	}
	return s.store.CreateList(ctx, id, name)
}
func (s *Service) RenameList(ctx context.Context, id, name string) (List, error) {
	name, err := validName(name)
	if err != nil {
		return List{}, err
	}
	return s.store.RenameList(ctx, id, name)
}
func (s *Service) RemoveItem(ctx context.Context, list, id string) error {
	return s.store.RemoveItem(ctx, list, id)
}
func (s *Service) ListDetail(ctx context.Context, id string) (ListDetail, error) {
	list, err := s.store.GetList(ctx, id)
	if err != nil {
		return ListDetail{}, err
	}
	items, err := s.store.GetItems(ctx, id)
	if err != nil {
		return ListDetail{}, err
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.PurchaseID)
	}
	evaluations, err := s.Evaluate(ctx, ids)
	if err != nil {
		return ListDetail{}, err
	}
	detail := ListDetail{List: list, Items: items}
	for i := range detail.Items {
		item := &detail.Items[i]
		e := evaluations[i]
		if e.Availability == Removed || e.Availability == Unknown {
			e.CardName = item.CardName
			e.CertNumber = item.CertNumber
			e.Grader = item.Grader
			e.Grade = item.Grade
		}
		item.Evaluation = e
		item.PriceChanged = item.AcknowledgedPriceCents != e.ListedPriceCents
		item.SupportChanged = item.AcknowledgedStatus != e.Status
		detail.Summary.TotalCount++
		if item.PackedAt != "" {
			detail.Summary.PackedCount++
		}
		switch e.Availability {
		case NotReceived:
			detail.Summary.NotReceivedCount++
		case Ready:
		default:
			detail.Summary.UnavailableCount++
		}
		if e.ListedPriceCents <= 0 {
			detail.Summary.MissingPriceCount++
		} else if e.PriceAssociationUnclear {
			detail.Summary.AmbiguousPriceCount++
		} else if e.Availability == Ready {
			detail.Summary.KnownValueCents += e.ListedPriceCents
		}
	}
	return detail, nil
}
func (s *Service) AddItems(ctx context.Context, list string, adds []AddItem) (ListDetail, error) {
	ids := make([]string, 0, len(adds))
	for _, a := range adds {
		ids = append(ids, a.PurchaseID)
	}
	// Persist association observations before a later optimistic conflict can roll back.
	if _, err := s.store.ObservePriceAssociations(ctx, ids); err != nil {
		return ListDetail{}, err
	}
	err := s.store.Within(ctx, func(tx Session) error {
		if _, err := tx.GetList(ctx, list); err != nil {
			return err
		}
		existing, err := tx.GetItems(ctx, list)
		if err != nil {
			return err
		}
		members := map[string]bool{}
		for _, item := range existing {
			members[item.PurchaseID] = true
		}
		if err := tx.LockPurchases(ctx, ids); err != nil {
			return err
		}
		es, err := evaluateBatch(ctx, tx, ids, s.now(), true)
		if err != nil {
			return err
		}
		for i, a := range adds {
			if members[a.PurchaseID] {
				continue
			}
			e := es[i]
			if !e.CanAdd || e.Version != a.EvaluationVersion {
				return ErrConflict
			}
			item := Item{ID: uuid.NewString(), PurchaseID: a.PurchaseID, CardName: e.CardName, CertNumber: e.CertNumber, Grader: e.Grader, Grade: e.Grade, AddedAt: Timestamp(s.now()), Version: 1, AcknowledgedPriceCents: e.ListedPriceCents, AcknowledgedStatus: e.Status}
			if err := tx.InsertItem(ctx, list, item); err != nil {
				return err
			}
			members[a.PurchaseID] = true
		}
		return nil
	})
	if err != nil {
		return ListDetail{}, err
	}
	return s.ListDetail(ctx, list)
}
func (s *Service) UpdateItem(ctx context.Context, list, id string, cmd UpdateItem) (ListDetail, error) {
	if cmd.Version <= 0 || (cmd.Packed == nil && !cmd.Acknowledge) {
		return ListDetail{}, ErrInvalid
	}
	needsReview := cmd.Acknowledge || (cmd.Packed != nil && *cmd.Packed)
	if needsReview {
		// Observation is its own commit: rejecting a stale pack/ack must not
		// forget a newly detected collision. The locked evaluation repeats it.
		items, err := s.store.GetItems(ctx, list)
		if err != nil {
			return ListDetail{}, err
		}
		for _, item := range items {
			if item.ID == id {
				if _, err := s.store.ObservePriceAssociations(ctx, []string{item.PurchaseID}); err != nil {
					return ListDetail{}, err
				}
				break
			}
		}
	}
	err := s.store.Within(ctx, func(tx Session) error {
		if _, err := tx.GetList(ctx, list); err != nil {
			return err
		}
		items, err := tx.GetItems(ctx, list)
		if err != nil {
			return err
		}
		var item Item
		for _, v := range items {
			if v.ID == id {
				item = v
				break
			}
		}
		if item.ID == "" {
			return ErrNotFound
		}
		var e Evaluation
		if needsReview {
			if err := tx.LockPurchases(ctx, []string{item.PurchaseID}); err != nil {
				return err
			}
			es, err := evaluateBatch(ctx, tx, []string{item.PurchaseID}, s.now(), true)
			if err != nil {
				return err
			}
			e = es[0]
			if e.Version != cmd.EvaluationVersion {
				return ErrConflict
			}
		}
		fingerprint := Fingerprint(cmd)
		if item.Version != cmd.Version {
			if item.LastCommand == fingerprint {
				return nil
			}
			return ErrConflict
		}
		if cmd.Packed != nil && *cmd.Packed && !e.CanPack {
			return ErrConflict
		}
		if needsReview {
			item.AcknowledgedPriceCents = e.ListedPriceCents
			item.AcknowledgedStatus = e.Status
		}
		if cmd.Packed != nil {
			if *cmd.Packed {
				if item.PackedAt == "" {
					item.PackedAt = Timestamp(s.now())
				}
			} else {
				item.PackedAt = ""
			}
		}
		item.LastCommand = fingerprint
		item.Version++
		return tx.SaveItem(ctx, list, item)
	})
	if err != nil {
		return ListDetail{}, err
	}
	return s.ListDetail(ctx, list)
}
