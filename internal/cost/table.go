package cost

import (
	"fmt"
	"strconv"
	"strings"
)

// Table renders the document the way togen cost prints it: a block per item with its lines
// and subtotal, the things left out, then the total with where the prices came from.
func (d Document) Table() []string {
	var nameW, kindW, labelW, qtyW, unitW, priceW, noteW int
	amountW := 7
	for _, item := range d.Items {
		nameW = max(nameW, len(item.Name))
		kindW = max(kindW, len(item.Kind))
		noteW = max(noteW, len(item.Note))
		amountW = max(amountW, len(money(item.Subtotal)))
		for _, l := range item.Lines {
			labelW = max(labelW, len(l.Label))
			qtyW = max(qtyW, len(quantity(l.Quantity)))
			unitW = max(unitW, len(l.Unit))
			priceW = max(priceW, len(unitPrice(l.UnitPrice)))
			amountW = max(amountW, len(money(l.Amount)))
		}
	}
	for _, o := range d.NotPriced {
		nameW = max(nameW, len(o.Name))
		kindW = max(kindW, len(o.Kind))
	}

	amountAt := max(2+labelW+2+qtyW+1+unitW+5+priceW+1+len(d.Currency)+2, 2+noteW+2)

	var out []string
	for _, item := range d.Items {
		out = append(out, strings.TrimRight(fmt.Sprintf("%-*s  %-*s  %s", nameW, item.Name, kindW, item.Kind, item.Summary), " "))
		if item.Note != "" {
			out = append(out, "  "+item.Note)
		}
		for _, l := range item.Lines {
			out = append(out, fmt.Sprintf("  %-*s  %*s %-*s  x  %*s %s  %*s",
				labelW, l.Label, qtyW, quantity(l.Quantity), unitW, l.Unit, priceW, unitPrice(l.UnitPrice), d.Currency,
				amountW, money(l.Amount)))
			if l.Note != "" {
				out = append(out, "    "+l.Note)
			}
		}
		out = append(out, strings.Repeat(" ", amountAt)+fmt.Sprintf("%*s", amountW, money(item.Subtotal)))
	}
	if len(d.NotPriced) > 0 {
		out = append(out, "not priced")
		for _, o := range d.NotPriced {
			out = append(out, fmt.Sprintf("  %-*s  %-*s  %s", nameW, o.Name, kindW, o.Kind, o.Reason))
		}
	}
	out = append(out, "")
	if d.Warning != "" {
		out = append(out, "warning: "+d.Warning)
	}
	return append(out, fmt.Sprintf("total  %s %s/month  %s, %s", money(d.Total), d.Currency, d.Region, d.Note))
}

func money(amount float64) string { return strconv.FormatFloat(amount, 'f', 2, 64) }

// Four places, or as many as a rate needs when four would show it as nothing: a Lambda
// request is 0.0000002.
func unitPrice(price float64) string {
	if price > 0 && price < 0.0001 {
		return strconv.FormatFloat(price, 'f', -1, 64)
	}
	return strconv.FormatFloat(price, 'f', 4, 64)
}

func quantity(q float64) string { return strconv.FormatFloat(q, 'f', -1, 64) }
