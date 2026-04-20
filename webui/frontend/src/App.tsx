import { startTransition, useDeferredValue, useEffect, useState } from "react";
import { Input } from "./components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./components/ui/select";
import { Badge } from "./components/ui/badge";

type FeedEvent = {
  id: string;
  sequence: number;
  timestamp: string;
  status: string;
  roleId?: string;
  playerName: string;
  itemId?: string;
  itemName: string;
  iconUrl?: string;
  result: "SUCCESS" | "FAILURE" | "RESET" | "DOWNGRADED" | "PICKUP" | "CRAFT" | "SENDMAIL";
  levelBefore: number;
  levelAfter: number;
  stoneId?: string;
  stoneName?: string;
  message: string;
  streakKey: string;
  srcRoleId?: string;
  dstRoleId?: string;
  srcPlayerName?: string;
  dstPlayerName?: string;
  money?: number;
  count?: number;
};

type RecentResponse = {
  events: FeedEvent[];
};

type TabType = "refine" | "craft" | "pickup" | "sendmail";

const EVENT_LIMIT = 400;
const PAGE_SIZE = 25;
const LOG_TIME_ZONE = "Asia/Jakarta";

function mergeEvents(current: FeedEvent[], incoming: FeedEvent[]): FeedEvent[] {
  const bySequence = new Map<number, FeedEvent>();
  for (const event of current) {
    bySequence.set(event.sequence, event);
  }
  for (const event of incoming) {
    bySequence.set(event.sequence, event);
  }

  return Array.from(bySequence.values())
    .sort((left, right) => left.sequence - right.sequence)
    .slice(-EVENT_LIMIT);
}

function embedTone(result: FeedEvent["result"]): string {
  switch (result) {
    case "SUCCESS":
      return "embed-success";
    case "FAILURE":
      return "embed-failure";
    case "RESET":
      return "embed-reset";
    case "DOWNGRADED":
      return "embed-downgraded";
    case "PICKUP":
      return "embed-pickup";
    case "CRAFT":
      return "embed-craft";
    case "SENDMAIL":
      return "embed-sendmail";
    default:
      return "embed-neutral";
  }
}

function authorLabel(event: FeedEvent): string {
  if (event.result === "PICKUP") {
    return event.itemName;
  }
  return event.itemName;
}

function normalizeSearchToken(value: string): string {
  return value.toLowerCase().replace(/\s+/g, " ").trim();
}

function normalizeLooseSearchToken(value: string): string {
  return normalizeSearchToken(value).replace(/\+/g, "");
}

function buildSearchHaystack(event: FeedEvent): string {
  if (event.result === "PICKUP") {
    return normalizeSearchToken(
      [
        event.playerName,
        event.itemName,
        event.message,
      ].join(" "),
    );
  }

  const levels = [
    String(event.levelBefore),
    String(event.levelAfter),
    `+${event.levelBefore}`,
    `+${event.levelAfter}`,
    `${event.levelBefore}->${event.levelAfter}`,
    `+${event.levelBefore}->+${event.levelAfter}`,
    `${event.levelBefore} -> ${event.levelAfter}`,
    `+${event.levelBefore} -> +${event.levelAfter}`,
    `from +${event.levelBefore}`,
    `to +${event.levelAfter}`,
  ];

  return normalizeSearchToken(
    [
      event.playerName,
      event.itemName,
      event.result,
      event.stoneName ?? "",
      event.message,
      ...levels,
    ].join(" "),
  );
}

function matchesSearch(event: FeedEvent, keyword: string): boolean {
  const haystack = buildSearchHaystack(event);
  if (haystack.includes(keyword)) {
    return true;
  }

  return normalizeLooseSearchToken(haystack).includes(normalizeLooseSearchToken(keyword));
}

type SystemCardProps = {
  author: string;
  description: string;
};

function SystemCard({ author, description }: SystemCardProps) {
  return (
    <article className="discord-embed embed-neutral">
      <p className="embed-author">{author}</p>
      <p className="embed-description">{description}</p>
    </article>
  );
}

type FeedProps = {
  events: FeedEvent[];
  tab: TabType;
  query: string;
  deferredQuery: string;
  connection: "loading" | "live" | "reconnecting";
  loadError: string;
  selectedResult: string;
  onResultChange: (value: string) => void;
  onQueryChange: (value: string) => void;
  levelAfterFilter: string;
  onLevelAfterChange: (value: string) => void;
};

function Feed({ events, tab, query, deferredQuery, connection, loadError, selectedResult, onResultChange, onQueryChange, levelAfterFilter, onLevelAfterChange }: FeedProps) {
  const [currentPage, setCurrentPage] = useState(1);
  const deferredQ = useDeferredValue(query);

  const orderedEvents = [...events].reverse();
  const keyword = deferredQ.trim().toLowerCase();
  const filteredEvents = orderedEvents.filter((event) => {
    if (tab === "refine" && (event.result === "PICKUP" || event.result === "CRAFT" || event.result === "SENDMAIL")) {
      return false;
    }
    if (tab === "craft" && event.result !== "CRAFT") {
      return false;
    }
    if (tab === "pickup" && event.result !== "PICKUP") {
      return false;
    }
    if (tab === "sendmail" && event.result !== "SENDMAIL") {
      return false;
    }
    if (selectedResult !== "ALL" && event.result !== selectedResult) {
      return false;
    }
    if (tab === "refine" && levelAfterFilter !== "" && event.levelAfter !== Number(levelAfterFilter)) {
      return false;
    }

    if (!keyword) {
      return true;
    }

    return matchesSearch(event, keyword);
  });
  const totalPages = Math.max(1, Math.ceil(filteredEvents.length / PAGE_SIZE));
  const page = Math.min(currentPage, totalPages);
  const pageStart = (page - 1) * PAGE_SIZE;
  const paginatedEvents = filteredEvents.slice(pageStart, pageStart + PAGE_SIZE);
  const pageNumbers = Array.from({ length: totalPages }, (_, index) => index + 1);
  const visibleFrom = filteredEvents.length === 0 ? 0 : pageStart + 1;
  const visibleTo = Math.min(pageStart + PAGE_SIZE, filteredEvents.length);

  const systemCard =
    loadError !== ""
      ? {
          author: "System",
          description: loadError,
        }
      : connection !== "live"
        ? {
            author: "Stream",
            description:
              connection === "loading"
                ? "Connecting to realtime feed..."
                : "Connection lost, attempting to reconnect...",
          }
        : orderedEvents.filter((e) =>
            tab === "refine" ? e.result !== "PICKUP" && e.result !== "CRAFT" && e.result !== "SENDMAIL" : tab === "craft" ? e.result === "CRAFT" : tab === "pickup" ? e.result === "PICKUP" : e.result === "SENDMAIL"
          ).length === 0
           ? {
               author: "System",
               description: tab === "refine"
                 ? "No refine events yet. Once logs come in, new embeds will appear here."
                 : tab === "craft"
                   ? "No craft events yet. Once logs come in, new embeds will appear here."
                   : tab === "pickup"
                     ? "No item pickup events yet. Once logs come in, new embeds will appear here."
                     : "No sendmail events yet. Once logs come in, new embeds will appear here.",
             }
          : null;
  const filterEmptyCard =
    filteredEvents.length === 0 && orderedEvents.length > 0
      ? {
          author: "Filter",
          description: "No events match your search or selected result filter.",
        }
      : null;

  useEffect(() => {
    setCurrentPage(1);
  }, [deferredQ, selectedResult, levelAfterFilter, tab]);

  useEffect(() => {
    if (currentPage > totalPages) {
      setCurrentPage(totalPages);
    }
  }, [currentPage, totalPages]);

  return (
    <div className="feed">
      <section className="toolbar">
        <label className="field field-grow">
          <span className="field-label">Search</span>
          <Input
            type="search"
            value={query}
            placeholder={tab === "refine" ? "Search player, item, or material..." : "Search player or item..."}
            onChange={(event) => onQueryChange(event.target.value)}
          />
        </label>

        <label className="field">
          <span className="field-label">Filter</span>
          <Select value={selectedResult} onValueChange={onResultChange}>
            <SelectTrigger>
              <SelectValue placeholder="Select filter" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="ALL">All</SelectItem>
              {tab === "refine" ? (
                <>
                  <SelectItem value="SUCCESS">Success</SelectItem>
                  <SelectItem value="FAILURE">Failure</SelectItem>
                  <SelectItem value="RESET">Reset</SelectItem>
                  <SelectItem value="DOWNGRADED">Downgraded</SelectItem>
                </>
              ) : (
                <SelectItem value="PICKUP">Pickup</SelectItem>
              )}
            </SelectContent>
          </Select>
        </label>

        {tab === "refine" && (
          <label className="field">
            <span className="field-label">Level After</span>
            <Select
              value={levelAfterFilter === "" ? "ANY" : levelAfterFilter}
              onValueChange={(value) => onLevelAfterChange(value === "ANY" ? "" : value)}
            >
              <SelectTrigger>
                <SelectValue placeholder="Any" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="ANY">Any</SelectItem>
                {Array.from({ length: 12 }, (_, i) => (
                  <SelectItem key={i + 1} value={String(i + 1)}>
                    {i + 1}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </label>
        )}
      </section>

      {systemCard ? <SystemCard author={systemCard.author} description={systemCard.description} /> : null}
      {filterEmptyCard ? <SystemCard author={filterEmptyCard.author} description={filterEmptyCard.description} /> : null}

      {filteredEvents.length > 0 ? (
        <>
          <section className="pagination">
            <p className="pagination-summary">
              Showing {visibleFrom}-{visibleTo} of {filteredEvents.length} events
            </p>

            <div className="pagination-actions">
              <button type="button" onClick={() => setCurrentPage((value) => Math.max(1, value - 1))} disabled={page === 1}>
                Prev
              </button>
              <div className="pagination-numbers">
                {pageNumbers.map((pageNumber) => (
                  <button
                    key={pageNumber}
                    type="button"
                    className={pageNumber === page ? "page-number is-active" : "page-number"}
                    onClick={() => setCurrentPage(pageNumber)}
                    aria-current={pageNumber === page ? "page" : undefined}
                  >
                    {pageNumber}
                  </button>
                ))}
              </div>
              <span className="pagination-page">
                Page {page}/{totalPages}
              </span>
              <button
                type="button"
                onClick={() => setCurrentPage((value) => Math.min(totalPages, value + 1))}
                disabled={page === totalPages}
              >
                Next
              </button>
            </div>
          </section>

          {paginatedEvents.map((event) => {
            const ts = new Date(event.timestamp);
            const dateStr = ts.toLocaleDateString("en-US", {
              day: "2-digit",
              month: "2-digit",
              year: "numeric",
              timeZone: LOG_TIME_ZONE,
            });
            const timeStr = ts.toLocaleTimeString("en-US", {
              hour: "2-digit",
              minute: "2-digit",
              second: "2-digit",
              hour12: false,
              timeZone: LOG_TIME_ZONE,
            });
            const isCraft = event.result === "CRAFT";
            return (
            <article key={event.id || event.sequence} className={`discord-embed ${embedTone(event.result)}`}>
              <div className="embed-author-row">
                {!isCraft && event.iconUrl ? (
                  <img
                    className="embed-icon"
                    src={event.iconUrl}
                    alt=""
                    width={16}
                    height={16}
                    loading="lazy"
                  />
                ) : null}
                <p className="embed-author">{isCraft ? "" : authorLabel(event)}</p>
                <span className="embed-timestamp">{dateStr} {timeStr}</span>
              </div>
              <p className="embed-description" style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                {isCraft && event.itemId ? (
                  <>
                    <img
                      className="embed-icon"
                      src={`/api/icons/${event.itemId}.png`}
                      alt=""
                      width={16}
                      height={16}
                      loading="lazy"
                      style={{ display: 'inline-block', verticalAlign: 'middle' }}
                    />
                    <span>{event.itemName} x{event.count} manufactured by {event.playerName}</span>
                  </>
                ) : (
                  event.message
                )}
              </p>
            </article>
            );
          })}
        </>
      ) : null}
    </div>
  );
}

export default function App() {
  const [events, setEvents] = useState<FeedEvent[]>([]);
  const [query, setQuery] = useState("");
  const [selectedResult, setSelectedResult] = useState("ALL");
  const [levelAfterFilter, setLevelAfterFilter] = useState("");
  const [activeTab, setActiveTab] = useState<TabType>("refine");
  const [connection, setConnection] = useState<"loading" | "live" | "reconnecting">("loading");
  const [loadError, setLoadError] = useState("");
  const deferredQuery = useDeferredValue(query);

  useEffect(() => {
    let mounted = true;
    const source = new EventSource("/api/events/stream");

    void fetch("/api/events/all")
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(`all events request failed: ${response.status}`);
        }
        return (await response.json()) as RecentResponse;
      })
      .then((payload) => {
        if (!mounted) {
          return;
        }
        startTransition(() => {
          setEvents((current) => mergeEvents(current, payload.events ?? []));
        });
        setLoadError("");
      })
      .catch((error: unknown) => {
        if (!mounted) {
          return;
        }
        setLoadError(error instanceof Error ? error.message : "failed to load recent events");
      });

    source.addEventListener("open", () => {
      if (!mounted) {
        return;
      }
      setConnection("live");
      setLoadError("");
    });

    source.addEventListener("error", () => {
      if (!mounted) {
        return;
      }
      setConnection("reconnecting");
    });

    source.addEventListener("refine", (event) => {
      if (!mounted) {
        return;
      }
      try {
        const parsed = JSON.parse(event.data);
        if (parsed == null || typeof parsed !== "object" || Array.isArray(parsed)) {
          return;
        }
        startTransition(() => {
          setEvents((current) => mergeEvents(current ?? [], [parsed as FeedEvent]));
        });
        setConnection("live");
      } catch {
        setLoadError("received an invalid realtime event");
      }
    });

    return () => {
      mounted = false;
      source.close();
    };
  }, []);

  return (
    <div className="app-shell">
      <nav className="tab-bar">
        <button
          type="button"
          className={`tab ${activeTab === "refine" ? "is-active" : ""}`}
          onClick={() => {
            setActiveTab("refine");
            setSelectedResult("ALL");
            setLevelAfterFilter("");
          }}
        >
          Refine
        </button>
        <button
          type="button"
          className={`tab ${activeTab === "craft" ? "is-active" : ""}`}
          onClick={() => {
            setActiveTab("craft");
            setSelectedResult("ALL");
            setLevelAfterFilter("");
          }}
        >
          Craft
        </button>
        <button
          type="button"
          className={`tab ${activeTab === "pickup" ? "is-active" : ""}`}
          onClick={() => {
            setActiveTab("pickup");
            setSelectedResult("ALL");
            setLevelAfterFilter("");
          }}
        >
          Pickup
        </button>
        <button
          type="button"
          className={`tab ${activeTab === "sendmail" ? "is-active" : ""}`}
          onClick={() => {
            setActiveTab("sendmail");
            setSelectedResult("ALL");
            setLevelAfterFilter("");
          }}
        >
          Sendmail
        </button>
      </nav>

      <Feed
        events={events}
        tab={activeTab}
        query={query}
        deferredQuery={deferredQuery}
        connection={connection}
        loadError={loadError}
        selectedResult={selectedResult}
        onResultChange={setSelectedResult}
        onQueryChange={setQuery}
        levelAfterFilter={levelAfterFilter}
        onLevelAfterChange={setLevelAfterFilter}
      />
    </div>
  );
}
