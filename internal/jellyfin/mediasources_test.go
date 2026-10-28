package jellyfin

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
