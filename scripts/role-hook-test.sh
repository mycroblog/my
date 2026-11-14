#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/role-hook.sh: which tool calls each role may make.
# The calls hold shell text for the hook to judge, never to expand here.
# shellcheck disable=SC2016
set -euo pipefail

hook="$(dirname "$0")/role-hook.sh"
fail=0

expect() {
  local want=$1 role=$2 call=$3 got=0
  "$hook" "$role" <<<"$call" 2>/dev/null || got=$?
  if [[ $got != "$want" ]]; then
    echo "FAIL: role-hook.sh $role, exit $got, want $want: $call" >&2
    fail=1
  fi
}

expect 0 builder '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/knowledge/visual/index.md"}}'
expect 2 builder '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/knowledge/visual-evaluator/index.md"}}'
expect 2 builder '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/agents/visual-evaluator.md"}}'
expect 2 builder '{"tool_name":"Bash","tool_input":{"command":"scripts/agentkit.sh show .claude/knowledge/visual-evaluator/decisions.md"}}'
expect 0 visual-evaluator '{"tool_name":"Read","tool_input":{"file_path":"/r/.tmp/visual/page/page-mobile.png"}}'
expect 0 visual-evaluator '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/knowledge/visual-evaluator/decisions.md"}}'
expect 0 visual-evaluator '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/knowledge/visual/decisions/color.md"}}'
expect 0 visual-evaluator '{"tool_name":"Read","tool_input":{"file_path":".claude/knowledge/visual/decisions/judging.md"}}'
expect 2 visual-evaluator '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/knowledge/visual/decisions/old/color.md"}}'
expect 2 visual-evaluator '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/knowledge/visual/index.md"}}'
expect 2 visual-evaluator '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/knowledge/visual/exemplars/color.md"}}'
expect 2 visual-evaluator '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/picks/visual.jsonl"}}'
expect 2 visual-evaluator '{"tool_name":"Read","tool_input":{"file_path":"/r/web/design-system/src/button.stories.tsx"}}'
expect 2 visual-evaluator '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/knowledge/visual-evaluator/../../agents/visual.md"}}'
expect 2 visual-evaluator '{"tool_name":"Bash","tool_input":{"command":"cat web/design-system/src/button.stories.tsx"}}'
expect 0 visual-evaluator '{"tool_name":"SubagentHandback","tool_input":{"message":"ship\n- none\nMust not break: the header"}}'
expect 2 visual-evaluator '{"tool_name":"SendMessage","tool_input":{"to":"main","message":"ship"}}'
expect 2 visual-evaluator '{"tool_name":"Grep","tool_input":{"pattern":"color","path":"/r/web"}}'
expect 2 visual-evaluator '{"tool_name":"Glob","tool_input":{"pattern":"/r/.claude/picks/*"}}'
expect 2 visual-evaluator '{"tool_name":"WebFetch","tool_input":{"url":"https://example.com"}}'
expect 2 visual-evaluator '{"tool_name":"Agent","tool_input":{"prompt":"read web/design-system/src/button.stories.tsx"}}'

# The test roles (ADR-0170 §2): the tests, their helpers, the issue, the ADRs and the public interface, never the
# implementation.
expect 0 go-test-developer '{"tool_name":"Read","tool_input":{"file_path":"/r/internal/app/content/post_test.go"}}'
expect 0 go-test-developer '{"tool_name":"Read","tool_input":{"file_path":"/r/pkg/testkit/clock.go"}}'
expect 0 go-test-developer '{"tool_name":"Read","tool_input":{"file_path":"/r/internal/interface/activitypub/testdata/note.json"}}'
expect 0 go-test-developer '{"tool_name":"Read","tool_input":{"file_path":"/r/docs/adr/0170-developer-roles.md"}}'
expect 0 go-test-developer '{"tool_name":"Read","tool_input":{"file_path":"/r/api/spec/openapi.yaml"}}'
expect 0 go-test-developer '{"tool_name":"Read","tool_input":{"file_path":"/r/pkg/mykit/api.gen.go"}}'
expect 0 go-test-developer '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/knowledge/go-test-developer/index.md"}}'
expect 2 go-test-developer '{"tool_name":"Read","tool_input":{"file_path":"/r/internal/app/content/post.go"}}'
expect 2 go-test-developer '{"tool_name":"Read","tool_input":{"file_path":"/r/pkg/mykit/client.go"}}'
expect 2 go-test-developer '{"tool_name":"Read","tool_input":{"file_path":"/r/pkg/testkit/../../internal/app/content/post.go"}}'
expect 2 go-test-developer '{"tool_name":"Read","tool_input":{"file_path":"/r/.claude/knowledge/go-developer/index.md"}}'
expect 0 go-test-developer '{"tool_name":"Edit","tool_input":{"file_path":"/r/internal/app/content/post_test.go"}}'
expect 0 go-test-developer '{"tool_name":"Write","tool_input":{"file_path":"/r/.claude/knowledge/go-test-developer/decisions/timing.md"}}'
expect 2 go-test-developer '{"tool_name":"Edit","tool_input":{"file_path":"/r/internal/app/content/post.go"}}'
expect 2 go-test-developer '{"tool_name":"Write","tool_input":{"file_path":"/r/docs/adr/0170-developer-roles.md"}}'
expect 0 go-test-developer '{"tool_name":"Grep","tool_input":{"pattern":"Clock","path":"/r/pkg/testkit"}}'
expect 2 go-test-developer '{"tool_name":"Grep","tool_input":{"pattern":"Clock","path":"/r/internal/app"}}'
expect 2 go-test-developer '{"tool_name":"Grep","tool_input":{"pattern":"Clock"}}'
expect 0 go-test-developer '{"tool_name":"Glob","tool_input":{"pattern":"internal/app/**/*.go"}}'
expect 2 go-test-developer '{"tool_name":"Agent","tool_input":{"prompt":"read internal/app/content/post.go"}}'
expect 0 go-test-developer '{"tool_name":"SubagentHandback","tool_input":{"message":"done"}}'
expect 0 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"gh issue view 3063 --repo mycroblog/my"}}'
expect 0 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"scripts/agentkit.sh outline internal/app/content/post.go"}}'
expect 0 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"go doc ./internal/app/content Service"}}'
expect 0 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"cd /r && GOWORK=off go test ./internal/app/content/ 2>&1 | tail -n 20"}}'
expect 0 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"go test ./... 2>&1 | grep -E '"'"'FAIL|panic'"'"' | head -n 5"}}'
expect 2 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"tail -n 20 internal/app/content/post.go"}}'
expect 2 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"grep -n Sleep internal/app/content/post.go"}}'
expect 0 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"scripts/agentkit.sh grep --only-tests '"'"'Clock\\|Sleep'"'"' internal"}}'
expect 0 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"scripts/agentkit.sh show internal/app/content/post_test.go:10-40"}}'
expect 0 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"git add internal/app/content/post_test.go && git commit -q -F - <<'"'"'EOF'"'"'\ntest(content): cover posts; see `Post` | done (#1)\n\nCo-Authored-By: x\nEOF"}}'
expect 0 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"scripts/task.sh go:lint >/dev/null"}}'
expect 2 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"go doc -src ./internal/app/content Service"}}'
expect 2 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"scripts/agentkit.sh show content.Service"}}'
expect 2 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"scripts/agentkit.sh show internal/app/content/post.go:1-40"}}'
expect 2 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"scripts/agentkit.sh grep Sleep internal"}}'
expect 2 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"scripts/agentkit.sh ctx 3063"}}'
expect 2 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"cat internal/app/content/post.go"}}'
expect 2 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"go test ./... && sed -n 1,40p internal/app/content/post.go"}}'
expect 2 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"go test ./...; git show HEAD"}}'
expect 2 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"go test \"$(cat internal/app/content/post.go)\""}}'
expect 2 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"gofmt internal/app/content/post.go > /tmp/x"}}'
expect 2 go-test-developer '{"tool_name":"Bash","tool_input":{"command":"go test < internal/app/content/post.go"}}'
expect 0 ts-test-developer '{"tool_name":"Read","tool_input":{"file_path":"/r/web/design-system/src/button.test.tsx"}}'
expect 0 ts-test-developer '{"tool_name":"Read","tool_input":{"file_path":"/r/web/app-data/src/test/fake.ts"}}'
expect 0 ts-test-developer '{"tool_name":"Read","tool_input":{"file_path":"/r/web/app-features/fixtures/layers.tsx"}}'
expect 0 ts-test-developer '{"tool_name":"Read","tool_input":{"file_path":"/r/web/e2e/src/screens.ts"}}'
expect 0 ts-test-developer '{"tool_name":"Read","tool_input":{"file_path":"/r/web/design-system/src/button.stories.tsx"}}'
expect 0 ts-test-developer '{"tool_name":"Read","tool_input":{"file_path":"/r/pkg/mykit/ts/src/schema.gen.ts"}}'
expect 2 ts-test-developer '{"tool_name":"Read","tool_input":{"file_path":"/r/web/design-system/src/button.tsx"}}'
expect 2 ts-test-developer '{"tool_name":"Read","tool_input":{"file_path":"/r/internal/app/content/post_test.go"}}'
expect 0 ts-test-developer '{"tool_name":"Edit","tool_input":{"file_path":"/r/web/design-system/src/button.test.tsx"}}'
expect 2 ts-test-developer '{"tool_name":"Edit","tool_input":{"file_path":"/r/web/design-system/src/button.stories.tsx"}}'
expect 2 ts-test-developer '{"tool_name":"Write","tool_input":{"file_path":"/r/web/design-system/src/button.tsx"}}'
expect 0 ts-test-developer '{"tool_name":"Bash","tool_input":{"command":"bun --bun vitest run web/design-system/src/button.test.tsx"}}'
expect 0 ts-test-developer '{"tool_name":"Bash","tool_input":{"command":"scripts/agentkit.sh outline web/design-system/src/button.tsx"}}'
expect 2 ts-test-developer '{"tool_name":"Bash","tool_input":{"command":"bun -e \"console.log(1)\""}}'
expect 2 nobody '{}'

exit "$fail"
