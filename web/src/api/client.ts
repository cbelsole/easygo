import type {
  AnnotateRequest,
  Analysis,
  AppConfig,
  GameList,
  GameView,
  JosekiInfo,
  JosekiReport,
  KataGoConfig,
  KataGoStatus,
  NewGameRequest,
  Point,
  ShapeFinding,
  User,
} from "./types";

export class ApiError extends Error {
  readonly status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, credentials: "same-origin" };
  if (body !== undefined) {
    init.headers = { "Content-Type": "application/json" };
    init.body = JSON.stringify(body);
  }
  const res = await fetch(path, init);
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  let data: unknown = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = null;
    }
  }
  if (!res.ok) {
    const msg =
      data && typeof data === "object" && "error" in data && typeof data.error === "string"
        ? data.error
        : `Request failed (${res.status})`;
    throw new ApiError(res.status, msg);
  }
  return data as T;
}

const get = <T>(path: string) => request<T>("GET", path);
const post = <T>(path: string, body?: unknown) => request<T>("POST", path, body ?? {});

/** Go encodes nil slices as null; make the arrays the UI iterates over always arrays. */
export function normalizeView(v: GameView): GameView {
  return {
    ...v,
    nodes: (v.nodes ?? []).map((n) => ({ ...n, children: n.children ?? [] })),
    mainLine: v.mainLine ?? [],
    dead: v.dead ?? [],
    board: v.board ?? [],
    liberties: v.liberties ?? [],
    territory: v.territory ?? [],
  };
}

const game = async (p: Promise<GameView>) => normalizeView(await p);
const g = (id: string) => `/api/games/${encodeURIComponent(id)}`;

export const api = {
  config: () => get<AppConfig>("/api/config"),
  me: () => get<{ user: User | null }>("/api/me"),
  updateMe: (p: { name: string; rank: string; location: string }) =>
    request<{ user: User }>("PUT", "/api/me", p),
  devLogin: (name: string) => post<User>("/api/auth/dev-login", { name }),
  logout: () => post<void>("/api/auth/logout"),

  listGames: async (): Promise<GameList> => {
    const l = await get<GameList>("/api/games");
    return { mine: l.mine ?? [], open: l.open ?? [] };
  },
  getGame: (id: string) => game(get<GameView>(g(id))),
  createGame: (req: NewGameRequest) => game(post<GameView>("/api/games", req)),
  importSgf: (sgf: string, mode: "teaching" | "joseki", name?: string) =>
    game(post<GameView>("/api/games/import", { sgf, mode, name })),
  josekiExample: () => game(post<GameView>("/api/joseki/example")),
  join: (id: string, color?: "black" | "white") =>
    game(post<GameView>(`${g(id)}/join`, color ? { color } : {})),
  play: (id: string, pt: Point) => game(post<GameView>(`${g(id)}/move`, pt)),
  pass: (id: string) => game(post<GameView>(`${g(id)}/move`, { pass: true })),
  resign: (id: string) => game(post<GameView>(`${g(id)}/resign`)),
  undo: (id: string) => game(post<GameView>(`${g(id)}/undo`)),
  goto: (id: string, node: number) => game(post<GameView>(`${g(id)}/goto`, { node })),
  annotate: (id: string, node: number, body: AnnotateRequest) =>
    game(post<GameView>(`${g(id)}/nodes/${node}/annotate`, body)),
  deleteNode: (id: string, node: number) =>
    game(request<GameView>("DELETE", `${g(id)}/nodes/${node}`)),
  promote: (id: string, node: number) => game(post<GameView>(`${g(id)}/nodes/${node}/promote`)),
  markDead: (id: string, pt: Point, dead: boolean) =>
    game(post<GameView>(`${g(id)}/dead`, { ...pt, dead })),
  accept: (id: string) => game(post<GameView>(`${g(id)}/accept`)),
  assist: (id: string, on: boolean) => game(post<GameView>(`${g(id)}/assist`, { on })),
  deleteGame: (id: string) => request<void>("DELETE", g(id)),
  sgfUrl: (id: string) => `${g(id)}/sgf`,

  analysis: (id: string, node: number, visits = 200) =>
    get<Analysis>(`${g(id)}/analysis?visits=${visits}&node=${node}`),
  shapes: async (id: string) =>
    (await get<{ findings: ShapeFinding[] | null }>(`${g(id)}/shapes`)).findings ?? [],
  joseki: async (id: string): Promise<JosekiReport> => {
    const r = await get<JosekiReport>(`${g(id)}/joseki`);
    return {
      loaded: r.loaded,
      corners: (r.corners ?? []).map((c) => ({ ...c, next: c.next ?? [] })),
    };
  },

  katago: () => get<KataGoStatus>("/api/katago"),
  setKatago: (c: KataGoConfig) => request<unknown>("PUT", "/api/katago", c),
  josekiInfo: () => get<JosekiInfo>("/api/joseki"),
  fetchJoseki: () => post<JosekiInfo>("/api/joseki/fetch"),
};

export function eventsUrl(id: string): string {
  return `${g(id)}/events`;
}
