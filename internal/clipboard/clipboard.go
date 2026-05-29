package clipboard

import "github.com/atotto/clipboard"

func Write(value string) error {
	return clipboard.WriteAll(value)
}
