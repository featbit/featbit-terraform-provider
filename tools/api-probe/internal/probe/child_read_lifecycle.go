package probe

import (
	"context"
	"errors"
	"net/http"
)

type ChildCollectionReadSummary struct {
	FeatureFlagDirectMissing       CompatibilityOutcome `json:"feature_flag_direct_missing"`
	FeatureFlagAbsentExactKeyCount int                  `json:"feature_flag_absent_exact_key_count"`
	SegmentDirectMissing           CompatibilityOutcome `json:"segment_direct_missing"`
	SegmentAbsentExactIDCount      int                  `json:"segment_absent_exact_id_count"`
	SegmentAbsentExactKeyCount     int                  `json:"segment_absent_exact_key_count"`
}

func lifecycleProbeChildCollections(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	environmentID string,
	prefix string,
) error {
	if report.ChildReads == nil {
		return errors.New("child-collection read summary is required")
	}
	if environmentID == "" {
		return errors.New(
			"child-collection reads require an owned environment",
		)
	}

	missingFlagKey := prefix
	flagDirectResult, flagDirectErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodGet,
		featureFlagPath(environmentID, missingFlagKey),
		featureFlagItemTemplate,
		nil,
	)
	flagMatch, err := lifecycleFindFeatureFlagByKeyAnyState(
		ctx,
		client,
		report,
		environmentID,
		missingFlagKey,
	)
	if err != nil {
		return errors.New(
			"feature-flag child-read exact-key fallback failed",
		)
	}
	report.ChildReads.FeatureFlagAbsentExactKeyCount = flagMatch.Count
	report.ChildReads.FeatureFlagDirectMissing = compatibilityOutcome(
		flagDirectResult,
		flagDirectErr,
		flagMatch.Count,
	)
	if flagMatch.Count != 0 {
		return errors.New(
			"owned environment unexpectedly contains the synthetic feature-flag key",
		)
	}
	if report.ChildReads.FeatureFlagDirectMissing.Classification ==
		ClassificationSuccess {
		return errors.New(
			"feature-flag direct Read and exact-key collection disagree",
		)
	}
	report.Workarounds = append(
		report.Workarounds,
		"feature_flag_direct_missing_all_page_exact_key_fallback",
	)

	missingSegmentID := deterministicProbeUUID(prefix + ":segment:missing")
	segmentDirectResult, segmentDirectErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodGet,
		segmentPath(environmentID, missingSegmentID),
		segmentItemTemplate,
		nil,
	)
	segmentIDMatch, err := lifecycleFindSegmentByID(
		ctx,
		client,
		report,
		environmentID,
		missingSegmentID,
	)
	if err != nil {
		return errors.New("segment child-read exact-ID fallback failed")
	}
	segmentKeyMatch, err := lifecycleFindSegmentByKey(
		ctx,
		client,
		report,
		environmentID,
		prefix,
	)
	if err != nil {
		return errors.New("segment child-read exact-key fallback failed")
	}
	report.ChildReads.SegmentAbsentExactIDCount = segmentIDMatch.Count
	report.ChildReads.SegmentAbsentExactKeyCount = segmentKeyMatch.Count
	report.ChildReads.SegmentDirectMissing = compatibilityOutcome(
		segmentDirectResult,
		segmentDirectErr,
		segmentIDMatch.Count,
	)
	if segmentIDMatch.Count != 0 || segmentKeyMatch.Count != 0 {
		return errors.New(
			"owned environment unexpectedly contains the synthetic segment identity",
		)
	}
	if report.ChildReads.SegmentDirectMissing.Classification ==
		ClassificationSuccess {
		return errors.New(
			"segment direct Read and exact-ID collection disagree",
		)
	}
	report.Workarounds = append(
		report.Workarounds,
		"segment_direct_missing_all_page_exact_id_fallback",
	)
	return nil
}
