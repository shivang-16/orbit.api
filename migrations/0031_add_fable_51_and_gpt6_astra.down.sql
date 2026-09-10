DELETE FROM model_catalogue
WHERE slug IN ('claude-fable-5-1', 'gpt-6-astra')
   OR model_id IN (
    'arn:aws:bedrock:ap-south-1:471112741644:inference-profile/global.anthropic.claude-fable-5-1',
    'arn:aws:bedrock:ap-south-1:471112741644:inference-profile/global.openai.gpt-6-astra',
    'arn:aws:bedrock:us-east-1:471112741644:inference-profile/global.anthropic.claude-fable-5-1',
    'arn:aws:bedrock:us-east-1:471112741644:inference-profile/global.openai.gpt-6-astra',
    'global.anthropic.claude-fable-5-1',
    'global.openai.gpt-6-astra'
);
