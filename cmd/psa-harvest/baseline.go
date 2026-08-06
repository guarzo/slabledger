package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/guarzo/slabledger/internal/platform/cardutil"
)

// parseBaselineFlag pulls -baseline-pull out of the raw CLI args before they
// reach config.Load(args). config.FromFlags (internal/platform/config/loader.go:236-262)
// builds its own flag.FlagSet defining only 9 flags (web, port, rate-limit,
// trust-proxy, log-level, log-json, cache, database-url, migrations-path) and
// fails with "flag provided but not defined" on anything else, so
// -baseline-pull must never reach it. rest is every arg config.Load is still
// allowed to see.
//
// A malformed value is a hard error, not a fallback. The tempting reading —
// "unparseable means false, and false is the safe default" — is backwards here:
// false is the *write* mode that drains the push queue to the portal. An
// operator who typed `-baseline-pull=ture` asked for a zero-write baseline pull
// and would instead get live portal writes, silently. Fail the run.
func parseBaselineFlag(args []string) (baseline bool, rest []string, err error) {
	rest = make([]string, 0, len(args))
	for _, a := range args {
		switch {
		case a == "-baseline-pull" || a == "--baseline-pull":
			baseline = true
		case len(a) > len("-baseline-pull=") && a[:len("-baseline-pull=")] == "-baseline-pull=":
			baseline, err = parseBoolFlag(a[len("-baseline-pull="):])
		case len(a) > len("--baseline-pull=") && a[:len("--baseline-pull=")] == "--baseline-pull=":
			baseline, err = parseBoolFlag(a[len("--baseline-pull="):])
		default:
			rest = append(rest, a)
			continue
		}
		if err != nil {
			return false, nil, err
		}
	}
	return baseline, rest, nil
}

// parseBoolFlag parses a CLI bool value. Garbage is an error: see the note on
// parseBaselineFlag for why falling back to false is the unsafe choice.
func parseBoolFlag(v string) (bool, error) {
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("invalid -baseline-pull value %q: want true or false", v)
	}
	return b, nil
}

// errNoSpecListName means none of the campaign's curated spec-list names
// mapped to a language token. This is the expected shape of the remaining
// CATEGORY-era campaigns (design doc §8): their edit form predates the
// curated-list model, so it names no "Japanese Pokemon" / "English Pokemon"
// list. They are not writable here; the operator converts them by hand in
// the portal and re-runs the baseline.
var errNoSpecListName = errors.New("no recognized curated spec-list name (see design §8: conversion path for CATEGORY campaigns)")

// errAmbiguousSpecListName means more than one distinct language mapped —
// writing either guess would be a coin flip on a live buying campaign.
var errAmbiguousSpecListName = errors.New("multiple recognized curated spec-list names present")

// baselineLanguage maps a portal campaign's curated spec-list names to the
// language token stored in inventory.Campaign.TargetLanguage. Exactly one
// distinct recognized name must be present.
func baselineLanguage(specListNames []string) (string, error) {
	token := ""
	for _, name := range specListNames {
		var candidate string
		switch name {
		case "Japanese Pokemon":
			candidate = cardutil.LangJapanese
		case "English Pokemon":
			candidate = cardutil.LangEnglish
		default:
			continue
		}
		if token != "" && token != candidate {
			return "", errAmbiguousSpecListName
		}
		token = candidate
	}
	if token == "" {
		return "", errNoSpecListName
	}
	return token, nil
}

// buildBaselineCampaign copies one portal campaign's targeting onto a copy of
// the internal campaign already linked to it. Subject and denied-spec ids are
// copied verbatim, never re-resolved by name: live portal ids span 4xxx/8xxx/
// 22xxx generations while getSubjects (used only for operator-added subjects,
// see (*psaportal.Client).FetchSubjects) returns only 22xxx ids, so
// name-based resolution here would silently rewrite ids on active,
// money-spending campaigns on the very next push.
func buildBaselineCampaign(existing inventory.Campaign, pc psacampaign.PortalCampaign) (inventory.Campaign, error) {
	lang, err := baselineLanguage(pc.SpecListNames)
	if err != nil {
		return inventory.Campaign{}, err
	}

	updated := existing
	updated.TargetLanguage = lang
	updated.SubjectFilterMode = pc.SubjectFilter.Type
	updated.Subjects = toTargetSubjects(pc.SubjectFilter.Subjects)
	updated.DeniedSpecs = toTargetSubjects(pc.DeniedSpecs)
	return updated, nil
}

// toTargetSubjects copies portal subject refs into the internal shape,
// preserving order and ids as-is.
func toTargetSubjects(refs []psacampaign.SubjectRef) []inventory.TargetSubject {
	out := make([]inventory.TargetSubject, len(refs))
	for i, r := range refs {
		out[i] = inventory.TargetSubject{ID: r.ID, Name: r.Name}
	}
	return out
}

// runBaselinePull copies live portal targeting into every linked, complete
// SlabLedger campaign row. It performs zero portal writes — the caller in
// main.go must return the result of this function directly, before ever
// reaching DrainPushQueue. Campaigns are skipped (not written) when they are
// not linked to a portal campaign, when the edit-form fetch that produced pc
// failed (TargetingComplete == false), or when the campaign's curated
// spec-list names don't map to exactly one language. Any skip is logged and
// makes the whole run return a non-zero-exit error, so a partial baseline is
// never mistaken for a complete one; the pull is idempotent, so re-running it
// is the remedy.
//
// The loop is driven by the portal's campaigns, so it can only report on rows
// the portal returned. That leaves one blind spot the loop cannot see by
// construction: an internal campaign carrying a PSACampaignRequestID that is
// absent from portalCampaigns is never visited at all, and would keep its stale
// pre-baseline targeting while the run exits 0. The unobserved-links check
// after the loop closes it.
func runBaselinePull(ctx context.Context, portalCampaigns []psacampaign.PortalCampaign, campaigns inventory.CampaignRepository, logger observability.Logger) error {
	internal, err := campaigns.ListCampaigns(ctx, false)
	if err != nil {
		return fmt.Errorf("baseline: list campaigns: %w", err)
	}
	byRequestID := make(map[string]inventory.Campaign, len(internal))
	for _, c := range internal {
		if c.PSACampaignRequestID != "" {
			byRequestID[c.PSACampaignRequestID] = c
		}
	}

	var skipped []string
	observed := make(map[string]bool, len(byRequestID))
	for _, pc := range portalCampaigns {
		existing, linked := byRequestID[pc.CampaignRequestID]
		if !linked {
			continue // no internal campaign to write to; the report step (not this function) flags these for the operator
		}
		observed[pc.CampaignRequestID] = true
		if !pc.TargetingComplete {
			logger.Warn(ctx, "psa-harvest: baseline skipping campaign, edit-form fetch was incomplete",
				observability.String("campaignId", existing.ID),
				observability.String("psaCampaignRequestId", pc.CampaignRequestID))
			skipped = append(skipped, existing.ID)
			continue
		}

		updated, err := buildBaselineCampaign(existing, pc)
		if err != nil {
			logger.Warn(ctx, "psa-harvest: baseline skipping campaign",
				observability.String("campaignId", existing.ID),
				observability.String("psaCampaignRequestId", pc.CampaignRequestID),
				observability.Err(err))
			skipped = append(skipped, existing.ID)
			continue
		}

		if err := campaigns.UpdateCampaign(ctx, &updated); err != nil {
			return fmt.Errorf("baseline: update campaign %s: %w", existing.ID, err)
		}
		logger.Info(ctx, "psa-harvest: baseline wrote campaign targeting",
			observability.String("campaignId", existing.ID),
			observability.String("targetLanguage", updated.TargetLanguage))
	}

	// Every internally linked campaign must have appeared in the portal fetch.
	// One that didn't is either a broken link or a campaign the portal dropped
	// from its list — both leave stale targeting behind, and neither is
	// something the operator should learn about from a silent exit 0.
	var unobserved []string
	for requestID, c := range byRequestID {
		if !observed[requestID] {
			logger.Warn(ctx, "psa-harvest: baseline found no portal campaign for a linked SlabLedger campaign",
				observability.String("campaignId", c.ID),
				observability.String("psaCampaignRequestId", requestID))
			unobserved = append(unobserved, c.ID)
		}
	}
	// Map iteration order is random; sort so the error message and its test
	// assertion are stable.
	sort.Strings(unobserved)

	switch {
	case len(skipped) > 0 && len(unobserved) > 0:
		return fmt.Errorf("baseline: %d campaign(s) skipped %v and %d linked campaign(s) missing from the portal %v, see warnings above",
			len(skipped), skipped, len(unobserved), unobserved)
	case len(skipped) > 0:
		return fmt.Errorf("baseline: %d campaign(s) skipped, see warnings above: %v", len(skipped), skipped)
	case len(unobserved) > 0:
		return fmt.Errorf("baseline: %d linked campaign(s) had no matching portal campaign, see warnings above: %v", len(unobserved), unobserved)
	}
	return nil
}
