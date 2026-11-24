-- SPDX-License-Identifier: AGPL-3.0-only

-- A revision with every part of it in one query (FindRevision), and the current revisions of many
-- statuses with theirs in one query (FindCurrentRevisions, for the pages that present many
-- statuses). Each part is a JSON array of its rows from a correlated subquery, in the order of the
-- part (an ordered subquery orders json_group_array); a part of at most one row is an array of
-- zero or one. A poll carries its options. The two queries read the same parts; keep them alike.

-- name: GetStatusRevision :one
SELECT r.id, r.status_id, r.seq, r.spoiler_text, r.sensitive, r.language, r.revised_at,
  CAST((SELECT json_group_array(json_object('language', b.language, 'content_html', b.content_html))
    FROM (SELECT language, content_html FROM status_revision_bodies WHERE revision_id = r.id ORDER BY language) b) AS TEXT) AS bodies,
  CAST((SELECT json_group_array(json_object('source_text', s.source_text, 'source_format', s.source_format))
    FROM status_revision_sources s WHERE s.revision_id = r.id) AS TEXT) AS sources,
  CAST((SELECT json_group_array(json_object('position', a.position, 'media_id', a.media_id, 'description', a.description))
    FROM (SELECT position, media_id, description FROM status_revision_attachments WHERE revision_id = r.id ORDER BY position) a) AS TEXT) AS attachments,
  CAST((SELECT json_group_array(m.account_id)
    FROM (SELECT account_id FROM status_revision_mentions WHERE revision_id = r.id ORDER BY account_id) m) AS TEXT) AS mentions,
  CAST((SELECT json_group_array(t.tag_id)
    FROM (SELECT tag_id FROM status_revision_tags WHERE revision_id = r.id ORDER BY tag_id) t) AS TEXT) AS tags,
  CAST((SELECT json_group_array(e.emoji_id)
    FROM (SELECT emoji_id FROM status_revision_emojis WHERE revision_id = r.id ORDER BY emoji_id) e) AS TEXT) AS emojis,
  CAST((SELECT json_group_array(json_object('id', p.id, 'multiple', p.multiple, 'hide_totals', p.hide_totals, 'closes_at', p.closes_at,
      'options', json((SELECT json_group_array(json_object('position', o.position, 'title', o.title))
        FROM (SELECT position, title FROM poll_options WHERE poll_id = p.id ORDER BY position) o))))
    FROM polls p WHERE p.revision_id = r.id) AS TEXT) AS polls,
  CAST((SELECT json_group_array(json_object('activity_uri', rm.activity_uri, 'html_policy_version', rm.html_policy_version,
      'omitted_attachment_count', rm.omitted_attachment_count))
    FROM remote_status_revisions rm WHERE rm.revision_id = r.id) AS TEXT) AS remotes,
  CAST((SELECT json_group_array(json_object('title', ar.title, 'summary', ar.summary, 'summary_origin', ar.summary_origin))
    FROM status_revision_articles ar WHERE ar.revision_id = r.id) AS TEXT) AS articles,
  CAST((SELECT json_group_array(json_object('media_id', c.media_id, 'description', c.description))
    FROM status_revision_article_covers c WHERE c.revision_id = r.id) AS TEXT) AS article_covers,
  CAST((SELECT json_group_array(json_object('position', i.position, 'media_id', i.media_id, 'description', i.description))
    FROM (SELECT position, media_id, description FROM status_revision_article_images WHERE revision_id = r.id ORDER BY position) i) AS TEXT) AS article_images
FROM status_revisions r WHERE r.id = sqlc.arg(id);

-- name: ListCurrentStatusRevisions :many
SELECT r.id, r.status_id, r.seq, r.spoiler_text, r.sensitive, r.language, r.revised_at,
  CAST((SELECT json_group_array(json_object('language', b.language, 'content_html', b.content_html))
    FROM (SELECT language, content_html FROM status_revision_bodies WHERE revision_id = r.id ORDER BY language) b) AS TEXT) AS bodies,
  CAST((SELECT json_group_array(json_object('source_text', s.source_text, 'source_format', s.source_format))
    FROM status_revision_sources s WHERE s.revision_id = r.id) AS TEXT) AS sources,
  CAST((SELECT json_group_array(json_object('position', a.position, 'media_id', a.media_id, 'description', a.description))
    FROM (SELECT position, media_id, description FROM status_revision_attachments WHERE revision_id = r.id ORDER BY position) a) AS TEXT) AS attachments,
  CAST((SELECT json_group_array(m.account_id)
    FROM (SELECT account_id FROM status_revision_mentions WHERE revision_id = r.id ORDER BY account_id) m) AS TEXT) AS mentions,
  CAST((SELECT json_group_array(t.tag_id)
    FROM (SELECT tag_id FROM status_revision_tags WHERE revision_id = r.id ORDER BY tag_id) t) AS TEXT) AS tags,
  CAST((SELECT json_group_array(e.emoji_id)
    FROM (SELECT emoji_id FROM status_revision_emojis WHERE revision_id = r.id ORDER BY emoji_id) e) AS TEXT) AS emojis,
  CAST((SELECT json_group_array(json_object('id', p.id, 'multiple', p.multiple, 'hide_totals', p.hide_totals, 'closes_at', p.closes_at,
      'options', json((SELECT json_group_array(json_object('position', o.position, 'title', o.title))
        FROM (SELECT position, title FROM poll_options WHERE poll_id = p.id ORDER BY position) o))))
    FROM polls p WHERE p.revision_id = r.id) AS TEXT) AS polls,
  CAST((SELECT json_group_array(json_object('activity_uri', rm.activity_uri, 'html_policy_version', rm.html_policy_version,
      'omitted_attachment_count', rm.omitted_attachment_count))
    FROM remote_status_revisions rm WHERE rm.revision_id = r.id) AS TEXT) AS remotes,
  CAST((SELECT json_group_array(json_object('title', ar.title, 'summary', ar.summary, 'summary_origin', ar.summary_origin))
    FROM status_revision_articles ar WHERE ar.revision_id = r.id) AS TEXT) AS articles,
  CAST((SELECT json_group_array(json_object('media_id', c.media_id, 'description', c.description))
    FROM status_revision_article_covers c WHERE c.revision_id = r.id) AS TEXT) AS article_covers,
  CAST((SELECT json_group_array(json_object('position', i.position, 'media_id', i.media_id, 'description', i.description))
    FROM (SELECT position, media_id, description FROM status_revision_article_images WHERE revision_id = r.id ORDER BY position) i) AS TEXT) AS article_images
FROM status_heads h JOIN status_revisions r ON r.id = h.revision_id
WHERE h.status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY r.status_id;
