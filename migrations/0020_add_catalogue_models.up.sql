-- Insert new catalogue models (DeepSeek, Mistral, Google, Meta, MiniMax, Qwen).
-- Idempotent: keeps existing UUIDs on re-run. Prices and context windows are
-- AWS Bedrock on-demand Standard, us-east-1, Aug 2026.
-- https://aws.amazon.com/bedrock/pricing/
-- https://docs.aws.amazon.com/bedrock/latest/userguide/model-cards.html

INSERT INTO model_catalogue (
    name, slug, vendor, provider, model_id, input_context_limit, sort_order, tags, modalities, is_active
) VALUES
    ('DeepSeek V3.2', 'deepseek-v3-2', 'deepseek', 'bedrock', 'deepseek.v3.2', 164000, 1, ARRAY['flagship','reasoning','coding','long-context']::text[], ARRAY['text']::text[], true),
    ('DeepSeek V3.1', 'deepseek-v3-1', 'deepseek', 'bedrock', 'deepseek.v3-v1:0', 128000, 2, ARRAY['flagship','reasoning','coding']::text[], ARRAY['text']::text[], true),
    ('DeepSeek R1', 'deepseek-r1', 'deepseek', 'bedrock', 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.deepseek.r1-v1:0', 128000, 3, ARRAY['flagship','reasoning','thinking','coding']::text[], ARRAY['text']::text[], true),
    ('Mistral Large 3', 'mistral-large-3', 'mistral', 'bedrock', 'mistral.mistral-large-3-675b-instruct', 256000, 1, ARRAY['flagship','reasoning','coding','long-context']::text[], ARRAY['text','image']::text[], true),
    ('Devstral 2 123B', 'devstral-2-123b', 'mistral', 'bedrock', 'mistral.devstral-2-123b', 256000, 2, ARRAY['coding','agentic','long-context']::text[], ARRAY['text']::text[], true),
    ('Magistral Small 2509', 'magistral-small-2509', 'mistral', 'bedrock', 'mistral.magistral-small-2509', 128000, 3, ARRAY['reasoning','thinking']::text[], ARRAY['text','image']::text[], true),
    ('Ministral 3 14B', 'ministral-3-14b', 'mistral', 'bedrock', 'mistral.ministral-3-14b-instruct', 128000, 4, ARRAY['balanced','lightweight']::text[], ARRAY['text','image']::text[], true),
    ('Ministral 3 8B', 'ministral-3-8b', 'mistral', 'bedrock', 'mistral.ministral-3-8b-instruct', 128000, 5, ARRAY['lightweight','cost-efficient']::text[], ARRAY['text','image']::text[], true),
    ('Ministral 3 3B', 'ministral-3-3b', 'mistral', 'bedrock', 'mistral.ministral-3-3b-instruct', 128000, 6, ARRAY['lightweight','fast','cost-efficient']::text[], ARRAY['text','image']::text[], true),
    ('Voxtral Small 24B', 'voxtral-small-24b', 'mistral', 'bedrock', 'mistral.voxtral-small-24b-2507', 32000, 7, ARRAY['balanced','audio']::text[], ARRAY['text','audio']::text[], true),
    ('Voxtral Mini 3B', 'voxtral-mini-3b', 'mistral', 'bedrock', 'mistral.voxtral-mini-3b-2507', 32000, 8, ARRAY['lightweight','fast','cost-efficient','audio']::text[], ARRAY['text','audio']::text[], true),
    ('Pixtral Large', 'pixtral-large', 'mistral', 'bedrock', 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.mistral.pixtral-large-2502-v1:0', 128000, 9, ARRAY['flagship','vision']::text[], ARRAY['text','image']::text[], true),
    ('Mistral Large 2402', 'mistral-large-2402', 'mistral', 'bedrock', 'mistral.mistral-large-2402-v1:0', 32000, 10, ARRAY['flagship','reasoning']::text[], ARRAY['text']::text[], true),
    ('Mistral Small 2402', 'mistral-small-2402', 'mistral', 'bedrock', 'mistral.mistral-small-2402-v1:0', 32000, 11, ARRAY['fast','cost-efficient']::text[], ARRAY['text']::text[], true),
    ('Mixtral 8x7B Instruct', 'mixtral-8x7b-instruct', 'mistral', 'bedrock', 'mistral.mixtral-8x7b-instruct-v0:1', 32000, 12, ARRAY['balanced','open-source']::text[], ARRAY['text']::text[], true),
    ('Mistral 7B Instruct', 'mistral-7b-instruct', 'mistral', 'bedrock', 'mistral.mistral-7b-instruct-v0:2', 32000, 13, ARRAY['lightweight','open-source','cost-efficient']::text[], ARRAY['text']::text[], true),
    ('Gemma 3 27B', 'gemma-3-27b', 'google', 'bedrock', 'google.gemma-3-27b-it', 128000, 1, ARRAY['flagship','open-source','balanced']::text[], ARRAY['text','image']::text[], true),
    ('Gemma 3 12B', 'gemma-3-12b', 'google', 'bedrock', 'google.gemma-3-12b-it', 128000, 2, ARRAY['balanced','open-source']::text[], ARRAY['text','image']::text[], true),
    ('Gemma 3 4B', 'gemma-3-4b', 'google', 'bedrock', 'google.gemma-3-4b-it', 128000, 3, ARRAY['lightweight','open-source','fast','cost-efficient']::text[], ARRAY['text','image']::text[], true),
    ('Llama 4 Maverick 17B', 'llama-4-maverick-17b', 'meta', 'bedrock', 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama4-maverick-17b-instruct-v1:0', 1000000, 1, ARRAY['flagship','balanced','long-context']::text[], ARRAY['text','image']::text[], true),
    ('Llama 4 Scout 17B', 'llama-4-scout-17b', 'meta', 'bedrock', 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama4-scout-17b-instruct-v1:0', 3500000, 2, ARRAY['long-context','cost-efficient']::text[], ARRAY['text','image']::text[], true),
    ('Llama 3.3 70B', 'llama-3-3-70b', 'meta', 'bedrock', 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama3-3-70b-instruct-v1:0', 128000, 3, ARRAY['balanced','reasoning']::text[], ARRAY['text']::text[], true),
    ('Llama 3.1 70B', 'llama-3-1-70b', 'meta', 'bedrock', 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama3-1-70b-instruct-v1:0', 128000, 4, ARRAY['balanced']::text[], ARRAY['text']::text[], true),
    ('Llama 3.1 8B', 'llama-3-1-8b', 'meta', 'bedrock', 'arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama3-1-8b-instruct-v1:0', 128000, 5, ARRAY['lightweight','fast','cost-efficient']::text[], ARRAY['text']::text[], true),
    ('Llama 3 70B', 'llama-3-70b', 'meta', 'bedrock', 'meta.llama3-70b-instruct-v1:0', 8000, 6, ARRAY['balanced']::text[], ARRAY['text']::text[], true),
    ('Llama 3 8B', 'llama-3-8b', 'meta', 'bedrock', 'meta.llama3-8b-instruct-v1:0', 8000, 7, ARRAY['lightweight','cost-efficient']::text[], ARRAY['text']::text[], true),
    ('MiniMax M2.5', 'minimax-m2-5', 'minimax', 'bedrock', 'minimax.minimax-m2.5', 196000, 1, ARRAY['flagship','agentic','coding','long-context']::text[], ARRAY['text']::text[], true),
    ('MiniMax M2.1', 'minimax-m2-1', 'minimax', 'bedrock', 'minimax.minimax-m2.1', 196000, 2, ARRAY['agentic','coding','long-context']::text[], ARRAY['text']::text[], true),
    ('MiniMax M2', 'minimax-m2', 'minimax', 'bedrock', 'minimax.minimax-m2', 1000000, 3, ARRAY['agentic','coding','long-context']::text[], ARRAY['text']::text[], true),
    ('Qwen3 Coder Next', 'qwen3-coder-next', 'qwen', 'bedrock', 'qwen.qwen3-coder-next', 256000, 1, ARRAY['flagship','coding','agentic','long-context']::text[], ARRAY['text']::text[], true),
    ('Qwen3 VL 235B A22B', 'qwen3-vl-235b-a22b', 'qwen', 'bedrock', 'qwen.qwen3-vl-235b-a22b', 256000, 2, ARRAY['flagship','vision','long-context']::text[], ARRAY['text','image']::text[], true),
    ('Qwen3 Next 80B A3B', 'qwen3-next-80b-a3b', 'qwen', 'bedrock', 'qwen.qwen3-next-80b-a3b', 256000, 3, ARRAY['balanced','reasoning','long-context']::text[], ARRAY['text']::text[], true),
    ('Qwen3 Coder 30B A3B', 'qwen3-coder-30b-a3b', 'qwen', 'bedrock', 'qwen.qwen3-coder-30b-a3b-v1:0', 256000, 4, ARRAY['coding','long-context','cost-efficient']::text[], ARRAY['text']::text[], true),
    ('Qwen3 32B', 'qwen3-32b', 'qwen', 'bedrock', 'qwen.qwen3-32b-v1:0', 32000, 5, ARRAY['reasoning','thinking','balanced']::text[], ARRAY['text']::text[], true)
ON CONFLICT (model_id) DO UPDATE SET
    name = EXCLUDED.name,
    slug = EXCLUDED.slug,
    vendor = EXCLUDED.vendor,
    provider = EXCLUDED.provider,
    input_context_limit = EXCLUDED.input_context_limit,
    sort_order = EXCLUDED.sort_order,
    tags = EXCLUDED.tags,
    modalities = EXCLUDED.modalities,
    is_active = EXCLUDED.is_active;

INSERT INTO model_pricing (
    model_catalogue_id,
    vendor_input_per_million_micros,
    vendor_output_per_million_micros
)
SELECT c.id, p.input_micros, p.output_micros
FROM (
    VALUES
        ('deepseek.v3.2', 620000::bigint, 1850000::bigint),
        ('deepseek.v3-v1:0', 620000, 1850000),
        ('arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.deepseek.r1-v1:0', 1350000, 5400000),
        ('mistral.mistral-large-3-675b-instruct', 500000, 1500000),
        ('mistral.devstral-2-123b', 400000, 2000000),
        ('mistral.magistral-small-2509', 500000, 1500000),
        ('mistral.ministral-3-14b-instruct', 200000, 200000),
        ('mistral.ministral-3-8b-instruct', 150000, 150000),
        ('mistral.ministral-3-3b-instruct', 100000, 100000),
        ('mistral.voxtral-small-24b-2507', 100000, 300000),
        ('mistral.voxtral-mini-3b-2507', 40000, 40000),
        ('arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.mistral.pixtral-large-2502-v1:0', 2000000, 6000000),
        ('mistral.mistral-large-2402-v1:0', 4000000, 12000000),
        ('mistral.mistral-small-2402-v1:0', 1000000, 3000000),
        ('mistral.mixtral-8x7b-instruct-v0:1', 450000, 700000),
        ('mistral.mistral-7b-instruct-v0:2', 150000, 200000),
        ('google.gemma-3-27b-it', 230000, 380000),
        ('google.gemma-3-12b-it', 90000, 290000),
        ('google.gemma-3-4b-it', 40000, 80000),
        ('arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama4-maverick-17b-instruct-v1:0', 240000, 970000),
        ('arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama4-scout-17b-instruct-v1:0', 170000, 660000),
        ('arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama3-3-70b-instruct-v1:0', 720000, 720000),
        ('arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama3-1-70b-instruct-v1:0', 720000, 720000),
        ('arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama3-1-8b-instruct-v1:0', 220000, 220000),
        ('meta.llama3-70b-instruct-v1:0', 2650000, 3500000),
        ('meta.llama3-8b-instruct-v1:0', 300000, 600000),
        ('minimax.minimax-m2.5', 300000, 1200000),
        ('minimax.minimax-m2.1', 300000, 1200000),
        ('minimax.minimax-m2', 300000, 1200000),
        ('qwen.qwen3-coder-next', 500000, 1200000),
        ('qwen.qwen3-vl-235b-a22b', 530000, 2660000),
        ('qwen.qwen3-next-80b-a3b', 150000, 1200000),
        ('qwen.qwen3-coder-30b-a3b-v1:0', 150000, 600000),
        ('qwen.qwen3-32b-v1:0', 150000, 600000)
) AS p(model_id, input_micros, output_micros)
JOIN model_catalogue c ON c.model_id = p.model_id
ON CONFLICT (model_catalogue_id) DO UPDATE SET
    vendor_input_per_million_micros = EXCLUDED.vendor_input_per_million_micros,
    vendor_output_per_million_micros = EXCLUDED.vendor_output_per_million_micros;
