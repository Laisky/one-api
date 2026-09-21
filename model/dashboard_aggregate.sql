WITH usage_groups AS /*MATERIALIZED*/ (
    SELECT created_at - ((created_at % 86400 + 86400) % 86400) AS day_start,
        type, model_name, COALESCE(model_name, '') AS tool_name, username, user_id, user_uuid, token_name,
        COUNT(*) AS request_count,
        SUM(quota) AS quota,
        SUM(CASE WHEN type = 2 THEN prompt_tokens ELSE 0 END) AS prompt_tokens,
        SUM(CASE WHEN type = 2 THEN completion_tokens ELSE 0 END) AS completion_tokens,
        SUM(CASE WHEN type = 2 THEN cached_prompt_tokens ELSE 0 END) AS cached_prompt_tokens,
        SUM(CASE WHEN type = 2 AND cached_prompt_tokens > 0 THEN 1 ELSE 0 END) AS cache_hit_count,
        SUM(CASE WHEN type = 2 AND cached_prompt_tokens > 0 THEN quota ELSE 0 END) AS cache_hit_quota
    FROM logs
    WHERE type IN (2, 7) AND created_at >= ? AND created_at < ? /*USER_FILTER*/
    GROUP BY day_start, type, model_name, tool_name, username, user_id, user_uuid, token_name
)
SELECT 0 AS kind, day_start, model_name,
    '' AS username, 0 AS user_id, '' AS user_uuid, '' AS token_name,
    SUM(request_count) AS request_count, COALESCE(SUM(quota), 0) AS quota,
    COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
    COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
    COALESCE(SUM(cached_prompt_tokens), 0) AS cached_prompt_tokens,
    COALESCE(SUM(cache_hit_count), 0) AS cache_hit_count,
    COALESCE(SUM(cache_hit_quota), 0) AS cache_hit_quota, 0 AS sort_user_id
FROM usage_groups WHERE type = 2
GROUP BY day_start, model_name
UNION ALL
SELECT 1, day_start, '', username, user_id, COALESCE(user_uuid, ''), '',
    SUM(request_count), COALESCE(SUM(quota), 0),
    COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0),
    COALESCE(SUM(cached_prompt_tokens), 0), COALESCE(SUM(cache_hit_count), 0),
    COALESCE(SUM(cache_hit_quota), 0), 0
FROM usage_groups WHERE type = 2
GROUP BY day_start, username, user_id, user_uuid
UNION ALL
SELECT 2, day_start, '', username, user_id, COALESCE(user_uuid, ''), COALESCE(token_name, ''),
    SUM(request_count), COALESCE(SUM(quota), 0),
    COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0),
    COALESCE(SUM(cached_prompt_tokens), 0), COALESCE(SUM(cache_hit_count), 0),
    COALESCE(SUM(cache_hit_quota), 0), 0
FROM usage_groups WHERE type = 2
GROUP BY day_start, username, user_id, user_uuid, token_name
UNION ALL
SELECT 3, day_start, tool_name,
    '', 0, '', '', SUM(request_count), COALESCE(SUM(quota), 0), 0, 0, 0, 0, 0, 0
FROM usage_groups WHERE type = 7
GROUP BY day_start, tool_name
UNION ALL
SELECT 4, day_start, '', COALESCE(username, ''), user_id, COALESCE(user_uuid, ''), '',
    SUM(request_count), COALESCE(SUM(quota), 0), 0, 0, 0, 0, 0, user_id
FROM usage_groups WHERE type = 7
GROUP BY day_start, username, user_id, user_uuid
UNION ALL
SELECT 5, day_start, '', COALESCE(username, ''), user_id, COALESCE(user_uuid, ''), COALESCE(token_name, ''),
    SUM(request_count), COALESCE(SUM(quota), 0), 0, 0, 0, 0, 0, user_id
FROM usage_groups WHERE type = 7
GROUP BY day_start, username, user_id, user_uuid, token_name
ORDER BY kind, day_start, username, sort_user_id, token_name, model_name, user_id, user_uuid
