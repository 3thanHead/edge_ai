# lead-gen (placeholder)

> 🚧 **Placeholder.** The real lead-generation app — LangChain pipelines for
> scraping, enriching, qualifying, and drafting outreach to leads — is the next
> project to build out here. This stub only proves the wiring to the LLM cluster.

It talks to the home [LLM cluster](../../infra/llm-cluster/) through the Mini PC's
single HAProxy endpoint using Ollama's OpenAI-compatible API — so as the cluster
load-balances and fails over across the Jetson / MacBook / Windows nodes, this app
never has to care which machine actually answers.

## Run the stub
```bash
python3 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
cp .env.example .env            # defaults point at http://192.168.1.111:11434/v1
python app/main.py
```
Expected: it prints a one-sentence answer from `llama3.2:3b` served by the cluster.
(Requires the cluster master + at least one node to be up.)

## Planned shape
- `app/` — LangChain chains/agents (sourcing → enrichment → qualification → drafting)
- swap the model or point `LLM_BASE_URL` elsewhere via `.env`, no code changes
