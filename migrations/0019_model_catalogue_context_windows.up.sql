-- Bedrock model-card context windows (AWS docs, Aug 2026).
-- Matched by model_id so existing catalogue UUIDs stay intact.

UPDATE model_catalogue SET input_context_limit = 1000000 WHERE model_id IN (
    'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-5',
    'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-sonnet-5',
    'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-fable-5',
    'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-4-8',
    'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-4-7',
    'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-sonnet-4-6',
    'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-4-6-v1',
    'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.openai.gpt-5.6-luna',
    'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.openai.gpt-5.6-sol',
    'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.openai.gpt-5.6-terra'
);

UPDATE model_catalogue SET input_context_limit = 200000 WHERE model_id IN (
    'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-4-5-20251101-v1:0',
    'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-haiku-4-5-20251001-v1:0',
    'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-sonnet-4-5-20250929-v1:0'
);

UPDATE model_catalogue SET input_context_limit = 128000 WHERE model_id IN (
    'openai.gpt-oss-safeguard-120b',
    'openai.gpt-oss-safeguard-20b',
    'openai.gpt-oss-120b-1:0',
    'openai.gpt-oss-20b-1:0'
);

UPDATE model_catalogue SET input_context_limit = 256000 WHERE model_id IN (
    'moonshotai.kimi-k2.5',
    'moonshot.kimi-k2-thinking'
);
