package models

// NormalizeRecs ensures JSON encodes [] instead of null for empty sections.
func NormalizeRecs(r PersonalizedRecs) PersonalizedRecs {
	return PersonalizedRecs{
		RecommendedForYou:   orEmptyRecs(r.RecommendedForYou),
		TrendingNow:         orEmptyRecs(r.TrendingNow),
		BecauseYouViewed:    orEmptyRecs(r.BecauseYouViewed),
		CartRecommendations: orEmptyRecs(r.CartRecommendations),
	}
}

func orEmptyRecs(s []Recommendation) []Recommendation {
	if s == nil {
		return []Recommendation{}
	}
	return s
}

func (r PersonalizedRecs) HasAny() bool {
	return len(r.RecommendedForYou) > 0 ||
		len(r.TrendingNow) > 0 ||
		len(r.BecauseYouViewed) > 0 ||
		len(r.CartRecommendations) > 0
}
