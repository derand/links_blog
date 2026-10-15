package web

import "strconv"

type pageLink struct {
	Title string
	URL   string // empty for the current page, gaps and disabled arrows
	Class string // "active" or "disabled"
}

// pagination builds « 1 … 4 5 [6] 7 8 … 12 » for a zero-based page, showing
// side pages around the current one. It reproduces the old site's layout.
func pagination(page, pages, side int, urlFor func(page int) string) []pageLink {
	var out []pageLink
	add := func(title string, i int) {
		out = append(out, pageLink{Title: title, URL: urlFor(i)})
	}
	addNum := func(i int) { add(strconv.Itoa(i+1), i) }
	gap := func() { out = append(out, pageLink{Title: "..."}) }

	if page == 0 {
		out = append(out, pageLink{Title: "«", Class: "disabled"})
	} else {
		add("«", page-1)
	}

	left, right := side, side
	if page < side {
		right += side - page
	}
	if page > pages-2-side {
		left += page - pages + 3
	}
	if page < left+3 {
		for i := 0; i < page; i++ {
			addNum(i)
		}
	} else {
		addNum(0)
		gap()
		for i := page - left; i < page; i++ {
			addNum(i)
		}
	}
	out = append(out, pageLink{Title: strconv.Itoa(page + 1), Class: "active"})
	if page > pages-right-4 {
		for i := page + 1; i < pages; i++ {
			addNum(i)
		}
	} else {
		for i := page + 1; i < page+1+right; i++ {
			addNum(i)
		}
		gap()
		addNum(pages - 1)
	}

	if page >= pages-1 {
		out = append(out, pageLink{Title: "»", Class: "disabled"})
	} else {
		add("»", page+1)
	}
	return out
}
