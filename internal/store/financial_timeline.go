package store

import "context"

const financialTimelineQuery = `
SELECT kind,tag,type_label,title,detail,person,amount_cent,sign,occurred_at,icon
FROM (
    SELECT 'expense' AS kind,'公共消费' AS tag,'' AS type_label,
        e.title AS title,
        TRIM(COALESCE(e.category,'') || CASE WHEN e.merchant<>'' THEN ' ' || e.merchant ELSE '' END || CASE WHEN e.description<>'' THEN ' · ' || e.description ELSE '' END || CASE WHEN e.payment_channel<>'' THEN CASE WHEN e.category<>'' OR e.merchant<>'' OR e.description<>'' THEN ' · ' ELSE '' END || e.payment_channel ELSE '' END) AS detail,
        payer.name AS person,e.amount_cent AS amount_cent,'−' AS sign,e.occurred_at AS occurred_at,'¥' AS icon,e.id AS id
    FROM public_expenses e JOIN members payer ON payer.id=e.handler_member_id
    WHERE e.status='active'
    UNION ALL
    SELECT 'transfer','成员转账','',
        sender.name || ' → ' || receiver.name,
        TRIM(COALESCE(t.purpose,'') || CASE WHEN t.payment_channel<>'' THEN CASE WHEN t.purpose<>'' THEN ' · ' ELSE '' END || t.payment_channel ELSE '' END),
        '',t.amount_cent,'',t.occurred_at,'⇄',t.id
    FROM holder_transfers t JOIN members sender ON sender.id=t.from_member_id JOIN members receiver ON receiver.id=t.to_member_id
    WHERE t.status='active'
    UNION ALL
    SELECT 'reimbursement','报销登记','',e.title,
        TRIM(COALESCE(r.payment_channel,'') || CASE WHEN r.note<>'' THEN CASE WHEN r.payment_channel<>'' THEN ' · ' ELSE '' END || r.note ELSE '' END),
        holder.name || ' → ' || receiver.name,r.amount_cent,'−',r.occurred_at,'↗',r.id
    FROM reimbursements r JOIN public_expenses e ON e.id=r.expense_id JOIN members holder ON holder.id=r.payer_holder_member_id JOIN members receiver ON receiver.id=r.receiver_member_id
    WHERE r.status='active' AND e.status='active'
    UNION ALL
    SELECT 'asset_event','资产变动',
        CASE e.event_type WHEN 'INITIAL_ASSET' THEN '初始资产' WHEN 'ASSET_IN' THEN '资产新增' WHEN 'ASSET_OUT' THEN '资产减少' ELSE '财务调整' END,
        CASE e.event_type WHEN 'INITIAL_ASSET' THEN '初始资产' WHEN 'ASSET_IN' THEN '资产新增' WHEN 'ASSET_OUT' THEN '资产划出' ELSE '财务调整' END,
        COALESCE(e.description,''),holder.name,ABS(e.amount_cent),
        CASE WHEN e.event_type='ASSET_OUT' OR (e.event_type='ADJUSTMENT' AND e.amount_cent<0) THEN '−' ELSE '+' END,
        e.occurred_at,'◈',e.id
    FROM asset_events e JOIN members holder ON holder.id=e.holder_member_id
    WHERE e.status='active'
) AS entries
ORDER BY julianday(occurred_at) DESC,occurred_at DESC,kind ASC,id DESC
LIMIT ? OFFSET ?`

func (s *Store) FinancialTimeline(ctx context.Context, page, pageSize int) (FinancialTimelinePage, error) {
	if pageSize < 1 {
		pageSize = 20
	}
	if page < 1 {
		page = 1
	}

	var result FinancialTimelinePage
	result.PageSize = pageSize
	err := s.DB.QueryRowContext(ctx, `
SELECT
    (SELECT COUNT(1) FROM public_expenses WHERE status='active') +
    (SELECT COUNT(1) FROM holder_transfers WHERE status='active') +
    (SELECT COUNT(1) FROM reimbursements r JOIN public_expenses e ON e.id=r.expense_id WHERE r.status='active' AND e.status='active') +
    (SELECT COUNT(1) FROM asset_events WHERE status='active')`).Scan(&result.Total)
	if err != nil {
		return result, err
	}
	result.Pages = (result.Total + pageSize - 1) / pageSize
	if result.Pages < 1 {
		result.Pages = 1
	}
	if page > result.Pages {
		page = result.Pages
	}
	result.Page = page
	result.HasPrevious = page > 1
	result.HasNext = page < result.Pages
	result.PreviousPage = page - 1
	result.NextPage = page + 1
	if result.Total > 0 {
		result.Start = (page-1)*pageSize + 1
		result.End = result.Start + pageSize - 1
		if result.End > result.Total {
			result.End = result.Total
		}
	}

	rows, err := s.DB.QueryContext(ctx, financialTimelineQuery, pageSize, (page-1)*pageSize)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	result.Items = make([]FinancialTimelineEntry, 0, pageSize)
	for rows.Next() {
		var item FinancialTimelineEntry
		if err := rows.Scan(&item.Kind, &item.Tag, &item.TypeLabel, &item.Title, &item.Detail, &item.Person, &item.AmountCent, &item.Sign, &item.OccurredAt, &item.Icon); err != nil {
			return result, err
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}
