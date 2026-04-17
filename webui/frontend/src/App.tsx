import { startTransition, useDeferredValue, useEffect, useState } from "react";

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
  result: "SUCCESS" | "FAILURE" | "RESET" | "DOWNGRADED";
  levelBefore: number;
  levelAfter: number;
  stoneId?: string;
  stoneName?: string;
  message: string;
  streakKey: string;
};

type RecentResponse = {
  events: FeedEvent[];
};

const EVENT_LIMIT = 400;
const PAGE_SIZE = 25;

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
    default:
      return "embed-neutral";
  }
}

function authorLabel(event: FeedEvent): string {
  return event.itemName;
}

function normalizeSearchToken(value: string): string {
  return value.toLowerCase().replace(/\s+/g, " ").trim();
}

function normalizeLooseSearchToken(value: string): string {
  return normalizeSearchToken(value).replace(/\+/g, "");
}

function buildSearchHaystack(event: FeedEvent): string {
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

export default function App() {
  const [events, setEvents] = useState<FeedEvent[]>([]);
  const [query, setQuery] = useState("");
  const [selectedResult, setSelectedResult] = useState("ALL");
  const [currentPage, setCurrentPage] = useState(1);
  const [connection, setConnection] = useState<"loading" | "live" | "reconnecting">("loading");
  const [loadError, setLoadError] = useState("");
  const deferredQuery = useDeferredValue(query);

  useEffect(() => {
    let mounted = true;
    const source = new EventSource("/api/events/stream");

    void fetch("/api/events/recent")
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(`recent request failed: ${response.status}`);
        }
        return (await response.json()) as RecentResponse;
      })
      .then((payload) => {
        if (!mounted) {
          return;
        }
        startTransition(() => {
          setEvents((current) => mergeEvents(current, payload.events));
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
        const parsed = JSON.parse(event.data) as FeedEvent;
        startTransition(() => {
          setEvents((current) => mergeEvents(current, [parsed]));
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

  const orderedEvents = [...events].reverse();
  const keyword = deferredQuery.trim().toLowerCase();
  const filteredEvents = orderedEvents.filter((event) => {
    if (selectedResult !== "ALL" && event.result !== selectedResult) {
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
                ? "Menghubungkan feed realtime..."
                : "Koneksi terputus, mencoba menyambung ulang...",
          }
        : orderedEvents.length === 0
          ? {
              author: "System",
              description: "Belum ada event refine. Begitu log masuk, embed baru akan muncul di sini.",
            }
          : null;
  const filterEmptyCard =
    filteredEvents.length === 0 && orderedEvents.length > 0
      ? {
          author: "Filter",
          description: "Tidak ada event yang cocok dengan pencarian atau hasil refine yang dipilih.",
        }
      : null;

  useEffect(() => {
    setCurrentPage(1);
  }, [deferredQuery, selectedResult]);

  useEffect(() => {
    if (currentPage > totalPages) {
      setCurrentPage(totalPages);
    }
  }, [currentPage, totalPages]);

  return (
    <div className="app-shell">
      <main className="feed">
        <section className="toolbar">
          <label className="field field-grow">
            <span className="field-label">Search</span>
            <input
              type="search"
              value={query}
              placeholder="Cari player, item, atau material..."
              onChange={(event) => setQuery(event.target.value)}
            />
          </label>

          <label className="field">
            <span className="field-label">Filter</span>
            <select value={selectedResult} onChange={(event) => setSelectedResult(event.target.value)}>
              <option value="ALL">Semua</option>
              <option value="SUCCESS">Success</option>
              <option value="FAILURE">Failure</option>
              <option value="RESET">Reset</option>
              <option value="DOWNGRADED">Downgraded</option>
            </select>
          </label>
        </section>

        {systemCard ? <SystemCard author={systemCard.author} description={systemCard.description} /> : null}
        {filterEmptyCard ? <SystemCard author={filterEmptyCard.author} description={filterEmptyCard.description} /> : null}

        {filteredEvents.length > 0 ? (
          <>
            <section className="pagination">
              <p className="pagination-summary">
                Menampilkan {visibleFrom}-{visibleTo} dari {filteredEvents.length} event
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

            {paginatedEvents.map((event) => (
            <article key={event.id || event.sequence} className={`discord-embed ${embedTone(event.result)}`}>
              <div className="embed-author-row">
                {event.iconUrl ? (
                  <img
                    className="embed-icon"
                    src={event.iconUrl}
                    alt=""
                    width={16}
                    height={16}
                    loading="lazy"
                  />
                ) : null}
                <p className="embed-author">{authorLabel(event)}</p>
              </div>
              <p className="embed-description">{event.message}</p>
            </article>
            ))}
          </>
        ) : null}
      </main>
    </div>
  );
}
