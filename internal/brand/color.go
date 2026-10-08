package brand

import (
	"fmt"
	"math"
	"strconv"
)

// Contrast is the WCAG 2 contrast ratio between two 6-digit hex colours.
func Contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(hex string) float64 {
	var ch [3]float64
	for i := range ch {
		v, _ := strconv.ParseUint(hex[2*i:2*i+2], 16, 8)
		c := float64(v) / 255
		if c <= 0.03928 {
			ch[i] = c / 12.92
		} else {
			ch[i] = math.Pow((c+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*ch[0] + 0.7152*ch[1] + 0.0722*ch[2]
}

// Darken returns the lightest shade of hex, scaled towards black, that
// reaches ratio against a white ground; "" when hex is not a colour. It is
// the suggestion an error about a pale accent can make: the same hue, as
// little darker as will do.
func Darken(hex string, ratio float64) string {
	if !hexRe.MatchString(hex) {
		return ""
	}
	var rgb [3]float64
	for i := range rgb {
		v, _ := strconv.ParseUint(hex[2*i:2*i+2], 16, 8)
		rgb[i] = float64(v)
	}
	for f := 1.0; f >= 0; f -= 0.01 {
		s := fmt.Sprintf("%02X%02X%02X", int(rgb[0]*f), int(rgb[1]*f), int(rgb[2]*f))
		if Contrast(s, "FFFFFF") >= ratio {
			return s
		}
	}
	return "000000"
}
