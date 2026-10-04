package metrics

import (
	"os/exec"
	"regexp"
	"strconv"
)

var pmsetPercent = regexp.MustCompile(`(\d+)%`)

func batteryPercent() (int, bool) {
	out, err := exec.Command("pmset", "-g", "batt").Output()
	if err != nil {
		return 0, false
	}
	m := pmsetPercent.FindSubmatch(out)
	if m == nil {
		return 0, false
	}
	v, err := strconv.Atoi(string(m[1]))
	return v, err == nil
}
