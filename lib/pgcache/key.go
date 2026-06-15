package pgcache

type StringKey string

func (s StringKey) Less(other StringKey) bool {
	return s < other
}

func (s StringKey) String() string {
	return string(s)
}

func StringKeyFromString(str string) (*StringKey, error) {
	sk := StringKey(str)
	return &sk, nil
}
