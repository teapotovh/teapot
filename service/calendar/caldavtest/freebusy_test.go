package caldavtest

import (
	"strconv"
	"testing"
	"time"
)

func TestFreeBusyVerifier(t *testing.T) {
	t.Parallel()

	year := strconv.Itoa(time.Now().Year())

	body := "BEGIN:VCALENDAR\n" +
		"BEGIN:VFREEBUSY\n" +
		"FREEBUSY:" + year + "0101T000000Z/" + year + "0101T020000Z\n" +
		"FREEBUSY;FBTYPE=BUSY-TENTATIVE:" + year + "0101T000000Z/" + year + "0102T000000Z\n" +
		"FREEBUSY;FBTYPE=BUSY-UNAVAILABLE:" + year + "0101T130000Z/PT2H\n" +
		"END:VFREEBUSY\n" +
		"END:VCALENDAR\n"

	assertFreeBusy(
		t,
		body,
		[]string{"0101T000000Z/0101T020000Z"},
		[]string{"0101T000000Z/0102T000000Z"},
		[]string{"0101T130000Z/0101T150000Z"},
	)
}
