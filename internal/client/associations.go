// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import "context"

// exactAssociation is the common safe shape shared by IAM relationship
// collections after each endpoint maps its differently named membership flag.
type exactAssociation struct {
	ID      string
	Present *bool
}

type exactAssociationPage struct {
	TotalCount *int64
	Items      []exactAssociation
}

// listCompleteAssociationIDs consumes a complete paginated relationship
// collection, requires every returned item to confirm membership, rejects
// duplicate UUIDs, and returns canonical IDs. Endpoint paths, query names,
// wire types, and decoding remain with the production caller.
func listCompleteAssociationIDs(
	ctx context.Context,
	operation string,
	parentID string,
	pageSize int,
	maxPageIndex int64,
	redactor *Redactor,
	listPage func(context.Context, int64) (exactAssociationPage, int, error),
) ([]string, error) {
	if !ValidUUID(parentID) {
		return nil, newAPIError(
			ClassificationValidation,
			0,
			operation,
			nil,
			redactor,
		)
	}

	pages := newCompletePageTracker(
		operation,
		pageSize,
		maxPageIndex,
		redactor.With(parentID),
	)
	associationIDs := make([]string, 0)
	for pageIndex := int64(0); ; pageIndex++ {
		page, statusCode, err := listPage(ctx, pageIndex)
		if err != nil {
			return nil, err
		}
		if err := pages.validatePage(
			page.TotalCount,
			page.Items != nil,
			len(page.Items),
			statusCode,
		); err != nil {
			return nil, err
		}
		for _, association := range page.Items {
			canonicalID, valid := CanonicalUUID(association.ID)
			if !valid || association.Present == nil || !*association.Present {
				return nil, newAPIError(
					ClassificationAmbiguous,
					statusCode,
					operation,
					nil,
					redactor.With(parentID, association.ID),
				)
			}
			if err := pages.recordExactID(
				canonicalID,
				statusCode,
				redactor.With(parentID, association.ID),
			); err != nil {
				return nil, err
			}
			associationIDs = append(associationIDs, canonicalID)
		}
		complete, err := pages.pageComplete(pageIndex, statusCode)
		if err != nil {
			return nil, err
		}
		if complete {
			return associationIDs, nil
		}
	}
}
