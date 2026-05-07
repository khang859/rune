package renameapi

func Double(s string) (int, error) {
	n, err := Parse(s)
	if err != nil {
		return 0, err
	}
	return n * 2, nil
}
