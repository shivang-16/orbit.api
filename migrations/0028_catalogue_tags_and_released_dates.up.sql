-- Backfill model_released_date and rewrite tags in place.
-- Matched by model_id so existing catalogue UUIDs, pricing rows,
-- and inference_request FKs stay intact. Idempotent: re-applying
-- (this migrator runs every .up.sql on each migrate) writes the
-- same values.

-- Claude Opus 5
UPDATE model_catalogue
SET tags = '{flagship,long-context,vision,reasoning,coding,agentic}',
    model_released_date = '2026-07-24'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-5';

-- Claude Sonnet 5
UPDATE model_catalogue
SET tags = '{balanced,long-context,vision,cost-efficient,coding,agentic}',
    model_released_date = '2026-06-30'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-sonnet-5';

-- Claude Fable 5
UPDATE model_catalogue
SET tags = '{flagship,long-context,vision,reasoning,agentic}',
    model_released_date = '2026-06-09'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-fable-5';

-- Claude Opus 4.8
UPDATE model_catalogue
SET tags = '{flagship,long-context,vision,reasoning,coding,agentic}',
    model_released_date = '2026-05-28'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-4-8';

-- Claude Opus 4.7
UPDATE model_catalogue
SET tags = '{flagship,long-context,vision,reasoning,coding,agentic}',
    model_released_date = '2026-04-16'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-4-7';

-- Claude Sonnet 4.6
UPDATE model_catalogue
SET tags = '{balanced,long-context,vision,cost-efficient,coding,agentic}',
    model_released_date = '2026-02-17'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-sonnet-4-6';

-- Claude Opus 4.6
UPDATE model_catalogue
SET tags = '{flagship,long-context,vision,reasoning,coding,agentic}',
    model_released_date = '2026-02-05'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-4-6-v1';

-- Claude Opus 4.5
UPDATE model_catalogue
SET tags = '{flagship,long-context,vision,reasoning,coding,agentic}',
    model_released_date = '2025-11-24'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-4-5-20251101-v1:0';

-- Claude Haiku 4.5
UPDATE model_catalogue
SET tags = '{lightweight,long-context,vision,cost-efficient,fast}',
    model_released_date = '2025-10-15'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-haiku-4-5-20251001-v1:0';

-- Claude Sonnet 4.5
UPDATE model_catalogue
SET tags = '{balanced,long-context,vision,cost-efficient,coding,agentic}',
    model_released_date = '2025-09-29'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-sonnet-4-5-20250929-v1:0';

-- DeepSeek V3.2
UPDATE model_catalogue
SET tags = '{balanced,cost-efficient,open-source,reasoning,coding,agentic}',
    model_released_date = '2025-12-01'
WHERE model_id = 'deepseek.v3.2';

-- DeepSeek V3.1
UPDATE model_catalogue
SET tags = '{lightweight,cost-efficient,open-source,coding}',
    model_released_date = '2025-08-21'
WHERE model_id = 'deepseek.v3-v1:0';

-- DeepSeek R1
UPDATE model_catalogue
SET tags = '{flagship,open-source,reasoning,coding}',
    model_released_date = '2025-01-20'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.deepseek.r1-v1:0';

-- Gemma 3 27B
UPDATE model_catalogue
SET tags = '{flagship,vision,open-source}',
    model_released_date = '2025-03-12'
WHERE model_id = 'google.gemma-3-27b-it';

-- Gemma 3 12B
UPDATE model_catalogue
SET tags = '{balanced,vision,cost-efficient,open-source}',
    model_released_date = '2025-03-12'
WHERE model_id = 'google.gemma-3-12b-it';

-- Gemma 3 4B
UPDATE model_catalogue
SET tags = '{lightweight,vision,cost-efficient,open-source,fast}',
    model_released_date = '2025-03-12'
WHERE model_id = 'google.gemma-3-4b-it';

-- Llama 4 Maverick 17B
UPDATE model_catalogue
SET tags = '{flagship,long-context,vision,open-source}',
    model_released_date = '2025-04-05'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama4-maverick-17b-instruct-v1:0';

-- Llama 4 Scout 17B
UPDATE model_catalogue
SET tags = '{balanced,long-context,vision,cost-efficient,open-source}',
    model_released_date = '2025-04-05'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama4-scout-17b-instruct-v1:0';

-- Llama 3.3 70B
UPDATE model_catalogue
SET tags = '{balanced,open-source}',
    model_released_date = '2024-12-06'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama3-3-70b-instruct-v1:0';

-- Llama 3.1 70B
UPDATE model_catalogue
SET tags = '{balanced,open-source}',
    model_released_date = '2024-07-23'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama3-1-70b-instruct-v1:0';

-- Llama 3.1 8B
UPDATE model_catalogue
SET tags = '{lightweight,cost-efficient,open-source,fast}',
    model_released_date = '2024-07-23'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama3-1-8b-instruct-v1:0';

-- Llama 3 70B
UPDATE model_catalogue
SET tags = '{balanced,open-source}',
    model_released_date = '2024-04-18'
WHERE model_id = 'meta.llama3-70b-instruct-v1:0';

-- Llama 3 8B
UPDATE model_catalogue
SET tags = '{lightweight,cost-efficient,open-source,fast}',
    model_released_date = '2024-04-18'
WHERE model_id = 'meta.llama3-8b-instruct-v1:0';

-- MiniMax M2.5
UPDATE model_catalogue
SET tags = '{flagship,cost-efficient,open-source,agentic,coding}',
    model_released_date = '2026-02-12'
WHERE model_id = 'minimax.minimax-m2.5';

-- MiniMax M2.1
UPDATE model_catalogue
SET tags = '{balanced,cost-efficient,open-source,agentic,coding}',
    model_released_date = '2025-12-22'
WHERE model_id = 'minimax.minimax-m2.1';

-- MiniMax M2
UPDATE model_catalogue
SET tags = '{lightweight,long-context,open-source,agentic,coding}',
    model_released_date = '2025-10-27'
WHERE model_id = 'minimax.minimax-m2';

-- Mistral Large 3
UPDATE model_catalogue
SET tags = '{flagship,long-context,vision,open-source,coding}',
    model_released_date = '2025-12-02'
WHERE model_id = 'mistral.mistral-large-3-675b-instruct';

-- Devstral 2 123B
UPDATE model_catalogue
SET tags = '{balanced,long-context,open-source,coding,agentic}',
    model_released_date = '2025-12-09'
WHERE model_id = 'mistral.devstral-2-123b';

-- Magistral Small 2509
UPDATE model_catalogue
SET tags = '{balanced,vision,open-source,reasoning}',
    model_released_date = '2025-09-01'
WHERE model_id = 'mistral.magistral-small-2509';

-- Ministral 3 14B
UPDATE model_catalogue
SET tags = '{lightweight,vision,cost-efficient,open-source}',
    model_released_date = '2025-12-02'
WHERE model_id = 'mistral.ministral-3-14b-instruct';

-- Ministral 3 8B
UPDATE model_catalogue
SET tags = '{lightweight,vision,cost-efficient,open-source}',
    model_released_date = '2025-12-02'
WHERE model_id = 'mistral.ministral-3-8b-instruct';

-- Ministral 3 3B
UPDATE model_catalogue
SET tags = '{lightweight,vision,cost-efficient,open-source,fast}',
    model_released_date = '2025-12-02'
WHERE model_id = 'mistral.ministral-3-3b-instruct';

-- Voxtral Small 24B
UPDATE model_catalogue
SET tags = '{balanced,audio,cost-efficient,open-source}',
    model_released_date = '2025-07-15'
WHERE model_id = 'mistral.voxtral-small-24b-2507';

-- Voxtral Mini 3B
UPDATE model_catalogue
SET tags = '{lightweight,audio,cost-efficient,open-source,fast}',
    model_released_date = '2025-07-15'
WHERE model_id = 'mistral.voxtral-mini-3b-2507';

-- Pixtral Large
UPDATE model_catalogue
SET tags = '{flagship,vision,open-source}',
    model_released_date = '2024-11-18'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.mistral.pixtral-large-2502-v1:0';

-- Mistral Large 2402
UPDATE model_catalogue
SET tags = '{flagship}',
    model_released_date = '2024-02-26'
WHERE model_id = 'mistral.mistral-large-2402-v1:0';

-- Mistral Small 2402
UPDATE model_catalogue
SET tags = '{balanced,fast}',
    model_released_date = '2024-02-26'
WHERE model_id = 'mistral.mistral-small-2402-v1:0';

-- Mixtral 8x7B Instruct
UPDATE model_catalogue
SET tags = '{balanced,open-source}',
    model_released_date = '2023-12-11'
WHERE model_id = 'mistral.mixtral-8x7b-instruct-v0:1';

-- Mistral 7B Instruct
UPDATE model_catalogue
SET tags = '{lightweight,cost-efficient,open-source,fast}',
    model_released_date = '2023-09-27'
WHERE model_id = 'mistral.mistral-7b-instruct-v0:2';

-- Kimi K2.5
UPDATE model_catalogue
SET tags = '{flagship,long-context,open-source,agentic}',
    model_released_date = '2026-01-27'
WHERE model_id = 'moonshotai.kimi-k2.5';

-- Kimi K2 Thinking
UPDATE model_catalogue
SET tags = '{balanced,long-context,cost-efficient,open-source,reasoning,agentic}',
    model_released_date = '2025-11-06'
WHERE model_id = 'moonshot.kimi-k2-thinking';

-- GPT 5.6 Luna
UPDATE model_catalogue
SET tags = '{lightweight,long-context,vision,fast}',
    model_released_date = '2026-07-09'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.openai.gpt-5.6-luna';

-- GPT 5.6 Sol
UPDATE model_catalogue
SET tags = '{flagship,long-context,vision,reasoning,agentic}',
    model_released_date = '2026-07-09'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.openai.gpt-5.6-sol';

-- GPT 5.6 Terra
UPDATE model_catalogue
SET tags = '{balanced,long-context,vision,agentic}',
    model_released_date = '2026-07-09'
WHERE model_id = 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.openai.gpt-5.6-terra';

-- GPT OSS Safeguard 120B
UPDATE model_catalogue
SET tags = '{balanced,cost-efficient,open-source,safety,reasoning}',
    model_released_date = '2025-10-29'
WHERE model_id = 'openai.gpt-oss-safeguard-120b';

-- GPT OSS Safeguard 20B
UPDATE model_catalogue
SET tags = '{lightweight,cost-efficient,open-source,safety,reasoning}',
    model_released_date = '2025-10-29'
WHERE model_id = 'openai.gpt-oss-safeguard-20b';

-- GPT OSS 120B
UPDATE model_catalogue
SET tags = '{balanced,open-source,reasoning,agentic}',
    model_released_date = '2025-08-05'
WHERE model_id = 'openai.gpt-oss-120b-1:0';

-- GPT OSS 20B
UPDATE model_catalogue
SET tags = '{lightweight,cost-efficient,open-source,reasoning,fast}',
    model_released_date = '2025-08-05'
WHERE model_id = 'openai.gpt-oss-20b-1:0';

-- Qwen3 Coder Next
UPDATE model_catalogue
SET tags = '{flagship,long-context,open-source,coding,agentic}',
    model_released_date = '2026-02-03'
WHERE model_id = 'qwen.qwen3-coder-next';

-- Qwen3 VL 235B A22B
UPDATE model_catalogue
SET tags = '{flagship,long-context,vision,open-source}',
    model_released_date = '2025-09-23'
WHERE model_id = 'qwen.qwen3-vl-235b-a22b';

-- Qwen3 Next 80B A3B
UPDATE model_catalogue
SET tags = '{balanced,long-context,open-source,reasoning}',
    model_released_date = '2025-09-11'
WHERE model_id = 'qwen.qwen3-next-80b-a3b';

-- Qwen3 Coder 30B A3B
UPDATE model_catalogue
SET tags = '{balanced,long-context,cost-efficient,open-source,coding}',
    model_released_date = '2025-07-22'
WHERE model_id = 'qwen.qwen3-coder-30b-a3b-v1:0';

-- Qwen3 32B
UPDATE model_catalogue
SET tags = '{lightweight,cost-efficient,open-source,reasoning}',
    model_released_date = '2025-04-28'
WHERE model_id = 'qwen.qwen3-32b-v1:0';
