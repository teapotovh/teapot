package backend

type ETagMatcher func(etag string) (bool, error)

func NegateETagMatch(matcher ETagMatcher) ETagMatcher {
	return func(etag string) (bool, error) {
		match, err := matcher(etag)
		return !match, err
	}
}

func AndETagMatch(matchers ...ETagMatcher) ETagMatcher {
	return func(etag string) (bool, error) {
		for _, matcher := range matchers {
			match, err := matcher(etag)
			if err != nil {
				return false, err
			}
			if !match {
				// Short cirtuit
				return false, nil
			}
		}

		return true, nil
	}
}
