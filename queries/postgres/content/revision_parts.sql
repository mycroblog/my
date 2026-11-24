-- SPDX-License-Identifier: AGPL-3.0-only

-- A revision with every part of it in one query (FindRevision), and the current revisions of many
-- statuses with theirs in one query (FindCurrentRevisions, for the pages that present many
-- statuses). Each part is a JSON array of its rows from a correlated subquery, in the order of the
-- part; a part of at most one row is an array of zero or one. A poll carries its options. The two
-- queries read the same parts; keep them alike.

-- name: GetStatusRevision :one
SELECT r.id, r.status_id, r.seq, r.spoiler_text, r.sensitive, r.language, r.revised_at,
  (SELECT coalesce(json_agg(json_build_object('language', b.language, 'content_html', b.content_html) ORDER BY b.language), '[]'::json)
    FROM status_revision_bodies b WHERE b.revision_id = r.id)::json AS bodies,
  (SELECT coalesce(json_agg(json_build_object('source_text', s.source_text, 'source_format', s.source_format)), '[]'::json)
    FROM status_revision_sources s WHERE s.revision_id = r.id)::json AS sources,
  (SELECT coalesce(json_agg(json_build_object('position', a.position, 'media_id', a.media_id, 'description', a.description) ORDER BY a.position), '[]'::json)
    FROM status_revision_attachments a WHERE a.revision_id = r.id)::json AS attachments,
  (SELECT coalesce(json_agg(m.account_id ORDER BY m.account_id), '[]'::json)
    FROM status_revision_mentions m WHERE m.revision_id = r.id)::json AS mentions,
  (SELECT coalesce(json_agg(t.tag_id ORDER BY t.tag_id), '[]'::json)
    FROM status_revision_tags t WHERE t.revision_id = r.id)::json AS tags,
  (SELECT coalesce(json_agg(e.emoji_id ORDER BY e.emoji_id), '[]'::json)
    FROM status_revision_emojis e WHERE e.revision_id = r.id)::json AS emojis,
  (SELECT coalesce(json_agg(json_build_object('id', p.id, 'multiple', p.multiple, 'hide_totals', p.hide_totals, 'closes_at', p.closes_at,
      'options', (SELECT coalesce(json_agg(json_build_object('position', o.position, 'title', o.title) ORDER BY o.position), '[]'::json)
        FROM poll_options o WHERE o.poll_id = p.id))), '[]'::json)
    FROM polls p WHERE p.revision_id = r.id)::json AS polls,
  (SELECT coalesce(json_agg(json_build_object('activity_uri', rm.activity_uri, 'html_policy_version', rm.html_policy_version,
      'omitted_attachment_count', rm.omitted_attachment_count)), '[]'::json)
    FROM remote_status_revisions rm WHERE rm.revision_id = r.id)::json AS remotes,
  (SELECT coalesce(json_agg(json_build_object('title', ar.title, 'summary', ar.summary, 'summary_origin', ar.summary_origin)), '[]'::json)
    FROM status_revision_articles ar WHERE ar.revision_id = r.id)::json AS articles,
  (SELECT coalesce(json_agg(json_build_object('media_id', c.media_id, 'description', c.description)), '[]'::json)
    FROM status_revision_article_covers c WHERE c.revision_id = r.id)::json AS article_covers,
  (SELECT coalesce(json_agg(json_build_object('position', i.position, 'media_id', i.media_id, 'description', i.description) ORDER BY i.position), '[]'::json)
    FROM status_revision_article_images i WHERE i.revision_id = r.id)::json AS article_images
FROM status_revisions r WHERE r.id = sqlc.arg(id)::uuid;

-- name: ListCurrentStatusRevisions :many
SELECT r.id, r.status_id, r.seq, r.spoiler_text, r.sensitive, r.language, r.revised_at,
  (SELECT coalesce(json_agg(json_build_object('language', b.language, 'content_html', b.content_html) ORDER BY b.language), '[]'::json)
    FROM status_revision_bodies b WHERE b.revision_id = r.id)::json AS bodies,
  (SELECT coalesce(json_agg(json_build_object('source_text', s.source_text, 'source_format', s.source_format)), '[]'::json)
    FROM status_revision_sources s WHERE s.revision_id = r.id)::json AS sources,
  (SELECT coalesce(json_agg(json_build_object('position', a.position, 'media_id', a.media_id, 'description', a.description) ORDER BY a.position), '[]'::json)
    FROM status_revision_attachments a WHERE a.revision_id = r.id)::json AS attachments,
  (SELECT coalesce(json_agg(m.account_id ORDER BY m.account_id), '[]'::json)
    FROM status_revision_mentions m WHERE m.revision_id = r.id)::json AS mentions,
  (SELECT coalesce(json_agg(t.tag_id ORDER BY t.tag_id), '[]'::json)
    FROM status_revision_tags t WHERE t.revision_id = r.id)::json AS tags,
  (SELECT coalesce(json_agg(e.emoji_id ORDER BY e.emoji_id), '[]'::json)
    FROM status_revision_emojis e WHERE e.revision_id = r.id)::json AS emojis,
  (SELECT coalesce(json_agg(json_build_object('id', p.id, 'multiple', p.multiple, 'hide_totals', p.hide_totals, 'closes_at', p.closes_at,
      'options', (SELECT coalesce(json_agg(json_build_object('position', o.position, 'title', o.title) ORDER BY o.position), '[]'::json)
        FROM poll_options o WHERE o.poll_id = p.id))), '[]'::json)
    FROM polls p WHERE p.revision_id = r.id)::json AS polls,
  (SELECT coalesce(json_agg(json_build_object('activity_uri', rm.activity_uri, 'html_policy_version', rm.html_policy_version,
      'omitted_attachment_count', rm.omitted_attachment_count)), '[]'::json)
    FROM remote_status_revisions rm WHERE rm.revision_id = r.id)::json AS remotes,
  (SELECT coalesce(json_agg(json_build_object('title', ar.title, 'summary', ar.summary, 'summary_origin', ar.summary_origin)), '[]'::json)
    FROM status_revision_articles ar WHERE ar.revision_id = r.id)::json AS articles,
  (SELECT coalesce(json_agg(json_build_object('media_id', c.media_id, 'description', c.description)), '[]'::json)
    FROM status_revision_article_covers c WHERE c.revision_id = r.id)::json AS article_covers,
  (SELECT coalesce(json_agg(json_build_object('position', i.position, 'media_id', i.media_id, 'description', i.description) ORDER BY i.position), '[]'::json)
    FROM status_revision_article_images i WHERE i.revision_id = r.id)::json AS article_images
FROM status_heads h JOIN status_revisions r ON r.id = h.revision_id
WHERE h.status_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY r.status_id;
