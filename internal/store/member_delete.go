package store

import "context"

const MemberDeleteSoft = "soft"

// MembersForAccounting returns members that must still participate in historical
// ledger calculations. Soft-deleted members remain here so deleting a member
// never changes public-asset totals or hides historical money records.
func (s *Store) MembersForAccounting(ctx context.Context) ([]Member, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT id,
       CASE WHEN is_del=1 THEN name || '（已删除）' ELSE name END,
       relation
FROM members
WHERE status='active' OR is_del=1
ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Member
	for rows.Next() {
		var v Member
		if err := rows.Scan(&v.ID, &v.Name, &v.Relation); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// DeleteMemberSmart is retained as the administrative delete entry point for
// compatibility, but deletion is now always a zero-balance soft delete.
func (s *Store) DeleteMemberSmart(ctx context.Context, auditActor, memberID int64) (string, error) {
	if err := s.SoftDeleteMember(ctx, auditActor, memberID); err != nil {
		return "", err
	}
	return MemberDeleteSoft, nil
}
