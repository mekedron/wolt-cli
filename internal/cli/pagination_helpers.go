package cli

import "fmt"

func resolvePageOffset(limit int, limitSet bool, offset int, offsetSet bool, page int, pageSet bool) (int, error) {
	if pageSet && offsetSet {
		return 0, fmt.Errorf("use either --offset or --page, not both")
	}
	if pageSet {
		if !limitSet || limit <= 0 {
			return 0, fmt.Errorf("--page requires --limit > 0")
		}
		if page < 1 {
			return 0, fmt.Errorf("--page must be >= 1")
		}
		return (page - 1) * limit, nil
	}
	if offset < 0 {
		return 0, nil
	}
	return offset, nil
}

func paginateFlatRows(data map[string]any, rowsKey string, limit *int, offset int) {
	if data == nil {
		return
	}
	rows := asSlice(data[rowsKey])
	total := len(rows)
	if offset < 0 {
		offset = 0
	}
	start := offset
	if start > total {
		start = total
	}
	end := total
	if limit != nil {
		if *limit < 0 {
			end = start
		} else if start+*limit < end {
			end = start + *limit
		}
	}
	data[rowsKey] = rows[start:end]
	data["total"] = total
	data["count"] = end - start
	data["offset"] = offset
	if limit != nil {
		data["limit"] = *limit
	}
	setTotalPages(data, total, limit)
	if end < total {
		data["next_offset"] = end
	} else {
		delete(data, "next_offset")
	}
}

func setTotalPages(data map[string]any, total int, limit *int) {
	if data == nil {
		return
	}
	if limit == nil || *limit <= 0 {
		delete(data, "total_pages")
		return
	}
	data["total_pages"] = (total + *limit - 1) / *limit
}
