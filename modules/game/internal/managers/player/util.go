package player

func Addition(count int32, addition int32) int32 {
	if addition <= 0 {
		return count
	}
	bonus := (count * addition) / 10000
	return count + bonus
}
