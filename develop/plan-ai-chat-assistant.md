# AI Chat Assistant & Native Go AI Implementation Plan

## Objective
Implement a global "AI Chat" feature accessible via a floating action button (FAB) in the `AppShell`. The chat will act as an intelligent assistant capable of understanding links, RSS feeds, text, and specific entities via auto-complete (`@Campaign`, `/commands`) to generate posts, configure automations, and create campaigns. 

Simultaneously, the existing Python `ai-service` will be completely and immediately replaced by a native Go module (`internal/ai/`). This eliminates complex migration phases, removes the need for token exchange between the application and the external AI server, and allows LLM API Keys and Endpoints to be managed securely per team.

## Scope & Impact
- **Backend Architecture (`api/`, `internal/ai/`):** 
  - **Immediate Python Removal:** The external Python `ai-service` is scrapped directly. No complex migration phase.
  - **Native Go AI Module:** All AI orchestration, prompt building, and LLM communication (OpenAI/Anthropic) are natively implemented in Go (`internal/ai/`).
  - **Per-Team Configuration:** API keys, models, and endpoints for LLM providers are configured and managed per team (using/extending the existing `AIServiceConfig`). The token exchange logic previously needed for the Python service is removed.
  - **Chat Orchestration:** Go orchestrates the chat loop, manages conversation history, parses commands, and invokes tools.
  - **Endpoints:** New API endpoints (e.g., `POST /v1/teams/{teamID}/ai/chat` via SSE) for streaming chat responses.
  - **Tools/Functions:** Go exposes domain functions directly to the LLM: `CreateDraft`, `CreateCampaign`, `CreateRecurringAutomation`, `CreateRSSAutomation`.
  - The AI feature is a modular Add-On in Go, activated via feature flags or team configuration.

- **Frontend (`frontend/src/`):** 
  - **Global AI Chat UI:** Introduction of an `AIChat` Drawer/Modal triggered by a FAB.
  - **ChatInput Component:** A rich text input with custom mentions support (Chips) to visually render `@Campaign` and `/commands`.
  - **Message Rendering:** Interactive chat cards for AI-generated posts with an "Open in Composer" action.
  - **Settings:** UI updates in team settings to input custom API keys and endpoints for the AI provider.

## Proposed Solution

### 1. Backend: Native Go Orchestration & Direct LLM Access
- **Team-Level Configuration:** Extend `AIServiceConfig` (or team settings) to store encrypted API keys and base URLs for OpenAI/Anthropic per team. Remove the old `ai-service` webhook authentication logic.
- **Go AI Module (`internal/ai/`):** Implement LLM API clients (e.g., using official Go SDKs or raw HTTP) to communicate directly with OpenAI or Anthropic. Port the essential prompt logic into Go text templates.
- **Agent Loop:** The Go backend orchestrates LLM interactions natively. It constructs system prompts using the team's `TeamProfile` and the dynamically referenced `@Campaign` context from the chat.
- **Tool Execution:** The LLM decides to call a tool -> Go executes the local domain function directly (e.g., inserting a row in the DB for a new campaign) -> Go returns the result to the LLM -> LLM formats the final response -> Streamed to the frontend via SSE.

### 2. Frontend: The Chat Interface
- **AppShell Integration:** Add a chat FAB. Clicking it opens a persistent but collapsible chat drawer.
- **ChatInput Component:** Implement an editor that detects `@` for entity selection (Campaigns, Automations, Profiles) and `/` for commands. Selected entities render as visual chips.
- **Composer Handoff:** AI-generated posts are rendered as a `PostPreviewCard` in the chat. An "Open in Composer" button seamlessly moves the content to the main Composer.

### 3. Tonal Guardrails
- The Go module will inject the team's `TeamProfile` (Brand Voice) into the system prompt.
- Guardrails emphasize natural phrasing, mandate brand vocabulary, and strictly forbid standard AI tropes ("In today's digital age...").

## Implementation Steps
1. **Remove Python Service:** Delete the `ai-service` directory and associated deployment configurations (Docker, Makefile).
2. **Data Models & API Updates:** 
   - Update Go domain models to handle Chat messages and Mentions.
   - Update team configuration APIs to accept LLM provider credentials securely.
   - Add the `POST /v1/teams/{teamID}/ai/chat` endpoint.
3. **Go Native AI Module (`internal/ai/`):** 
   - Implement direct LLM communication.
   - Recreate the brand voice prompt builder natively in Go.
   - Implement the tool-calling orchestration loop.
4. **Frontend UI:** 
   - Build the FAB and Chat Drawer.
   - Implement the Mention-capable ChatInput.
   - Wire up the SSE stream to render chat messages and interactive cards.
5. **Testing & Polish:** 
   - Ensure tools (drafting, campaigns) are triggered correctly by the LLM.
   - Verify prompt adherence to brand tonality.
