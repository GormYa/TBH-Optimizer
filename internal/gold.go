package internal

func ExtractGold(coins []Currency) int {
	for _, coin := range coins {
		if coin.Key == 100001 {
			return coin.Quantity
		}
	}
	return 0
}
