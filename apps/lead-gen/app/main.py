#!/usr/bin/env python3
"""lead-gen — PLACEHOLDER.

The real lead-generation app (LangChain pipelines: scrape -> enrich -> qualify ->
draft outreach) comes next. For now this stub just proves an app can reach the
home LLM cluster through the Mini PC's HAProxy endpoint using the OpenAI-compatible
API that Ollama exposes.

    python app/main.py
"""
import os

from langchain_openai import ChatOpenAI

# The cluster's single endpoint (HAProxy -> whichever Ollama node is up).
BASE_URL = os.environ.get("LLM_BASE_URL", "http://192.168.1.111:11434/v1")
MODEL = os.environ.get("LLM_MODEL", "llama3.2:3b")


def main():
    llm = ChatOpenAI(
        base_url=BASE_URL,
        api_key=os.environ.get("LLM_API_KEY", "ollama"),  # Ollama ignores the key
        model=MODEL,
        temperature=0.3,
    )
    print(f"[lead-gen] asking {MODEL} via {BASE_URL} …")
    resp = llm.invoke("In one sentence, what makes a strong B2B sales lead?")
    print(f"[lead-gen] reply: {resp.content}")


if __name__ == "__main__":
    main()
