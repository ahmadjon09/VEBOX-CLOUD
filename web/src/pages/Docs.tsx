import { useState } from "react";
import { ExternalLink } from "lucide-react";
import { useI18n } from "../i18n/context";
import { apiUrl } from "../services/api";
import { Seo } from "../components/Seo";
import { Badge, Copy, Micro } from "../components/ui";

type Method = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

interface Ep {
  m: Method;
  path: string;
  k: string;
  dk: string;
  auth: string;
  sample: string;
}

const EPS: Ep[] = [
  { m: "POST", path: "/v1/auth/signup", k: "dc.ep.signup.t", dk: "dc.ep.signup.d", auth: "—", sample: "curl -X POST /v1/auth/signup -d '{\"email\":\"a@b.c\",\"password\":\"min8belgi\"}'" },
  { m: "POST", path: "/v1/auth/login", k: "dc.ep.login.t", dk: "dc.ep.login.d", auth: "—", sample: "curl -X POST /v1/auth/login -d '{\"email\":\"a@b.c\",\"password\":\"...\"}'" },
  { m: "GET", path: "/auth/session", k: "dc.ep.session.t", dk: "dc.ep.session.d", auth: "Cookie/Bearer", sample: "curl /auth/session" },
  { m: "POST", path: "/v1/images", k: "dc.ep.upload.t", dk: "dc.ep.upload.d", auth: "Key/Bearer", sample: "curl -H 'X-API-Key: …' -F file=@photo.jpg /v1/images" },
  { m: "GET", path: "/v1/images", k: "dc.ep.list.t", dk: "dc.ep.list.d", auth: "Key/Bearer", sample: "curl -H 'X-API-Key: …' '/v1/images?limit=60&sort=newest'" },
  { m: "GET", path: "/v1/images/details", k: "dc.ep.details.t", dk: "dc.ep.details.d", auth: "Key/Bearer", sample: "curl -H 'X-API-Key: …' '/v1/images/details?ids=k7m2xQ9a,9bQ2xK7m'" },
  { m: "GET", path: "/v1/images/{id}", k: "dc.ep.meta.t", dk: "dc.ep.meta.d", auth: "Key/Bearer", sample: "curl -H 'X-API-Key: …' /v1/images/k7m2xQ9a" },
  { m: "PATCH", path: "/v1/images/{id}", k: "dc.ep.vis.t", dk: "dc.ep.vis.d", auth: "Key/Bearer", sample: "curl -X PATCH -d '{\"visibility\":\"public\"}' /v1/images/k7m2xQ9a" },
  { m: "DELETE", path: "/v1/images/{id}", k: "dc.ep.del.t", dk: "dc.ep.del.d", auth: "Key/Bearer", sample: "curl -X DELETE -H 'X-API-Key: …' /v1/images/k7m2xQ9a" },
  { m: "DELETE", path: "/v1/images?ids=a,b", k: "dc.ep.bulkdel.t", dk: "dc.ep.bulkdel.d", auth: "Key/Bearer", sample: "curl -X DELETE -H 'X-API-Key: …' '/v1/images?ids=k7m2xQ9a,9bQ2xK7m'" },
  { m: "PATCH", path: "/v1/images?ids=a,b", k: "dc.ep.bulkvis.t", dk: "dc.ep.bulkvis.d", auth: "Key/Bearer", sample: "curl -X PATCH -H 'X-API-Key: …' '/v1/images?ids=k7m2xQ9a&visibility=private'" },
  { m: "POST", path: "/v1/images/{id}/share", k: "dc.ep.share.t", dk: "dc.ep.share.d", auth: "Key/Bearer", sample: "curl -X POST -d '{\"ttl_seconds\":86400}' /v1/images/k7m2xQ9a/share" },
  { m: "GET", path: "/v1/me", k: "dc.ep.me.t", dk: "dc.ep.me.d", auth: "Key/Bearer", sample: "curl -H 'X-API-Key: …' /v1/me" },
  { m: "GET", path: "/v1/analytics", k: "dc.ep.analytics.t", dk: "dc.ep.analytics.d", auth: "Key/Bearer", sample: "curl -H 'X-API-Key: …' '/v1/analytics?days=30'" },
  { m: "GET", path: "/v1/keys", k: "dc.ep.keys.t", dk: "dc.ep.keys.d", auth: "Key/Bearer", sample: "curl -H 'X-API-Key: …' /v1/keys" },
  { m: "POST", path: "/v1/keys", k: "dc.ep.keynew.t", dk: "dc.ep.keynew.d", auth: "Key/Bearer", sample: "curl -X POST -d '{\"name\":\"ci\"}' /v1/keys" },
  { m: "DELETE", path: "/v1/keys/{id}", k: "dc.ep.keydel.t", dk: "dc.ep.keydel.d", auth: "Key/Bearer", sample: "curl -X DELETE -H 'X-API-Key: …' /v1/keys/ki_abc" },
  { m: "GET", path: "/cdn/{id}", k: "dc.ep.cdn.t", dk: "dc.ep.cdn.d", auth: "Public", sample: "curl /cdn/k7m2xQ9a" },
  { m: "GET", path: "/cdn/{id}?w=640", k: "dc.ep.cdnw.t", dk: "dc.ep.cdnw.d", auth: "Public", sample: "curl '/cdn/k7m2xQ9a?w=640' -o out.jpg" },
  { m: "GET", path: "/preview/{id}", k: "dc.ep.preview.t", dk: "dc.ep.preview.d", auth: "Public", sample: "curl /preview/k7m2xQ9a" },
  { m: "GET", path: "/i/{id}", k: "dc.ep.stream.t", dk: "dc.ep.stream.d", auth: "Key/Bearer", sample: "curl -H 'X-API-Key: …' '/i/k7m2xQ9a?v=hd&w=800'" },
  { m: "GET", path: "/d/{id}", k: "dc.ep.dl.t", dk: "dc.ep.dl.d", auth: "Key/Bearer", sample: "curl -H 'X-API-Key: …' -OJ /d/k7m2xQ9a" },
  { m: "GET", path: "/s/{id}?exp=…&sig=…", k: "dc.ep.signed.t", dk: "dc.ep.signed.d", auth: "Signed", sample: "curl '/s/k7m2xQ9a?exp=1760000000&sig=…'" },
  { m: "GET", path: "/v1/status", k: "dc.ep.status.t", dk: "dc.ep.status.d", auth: "—", sample: "curl /v1/status" },
  { m: "GET", path: "/openapi.json", k: "dc.ep.openapi.t", dk: "dc.ep.openapi.d", auth: "—", sample: "curl /openapi.json" },
];

const METHOD_COLOR: Record<Method, string> = {
  GET: "text-ok",
  POST: "text-fg",
  PUT: "text-warn",
  PATCH: "text-warn",
  DELETE: "text-err",
};

function CodeLine({ code, label }: { code: string; label: string }) {
  return (
    <div className="flex items-center justify-between gap-2 border border-line bg-ink px-3 py-2.5">
      <code className="min-w-0 flex-1 truncate text-[11px] text-mut" title={code}>{code}</code>
      <span className="micro hidden shrink-0 text-dim sm:inline">{label}</span>
      <Copy text={code} />
    </div>
  );
}

export function Docs() {
  const { t } = useI18n();
  const [filter, setFilter] = useState("");

  const rows = EPS.map((e) => ({ ...e, title: t(e.k), desc: t(e.dk) })).filter((e) => {
    const f = filter.trim().toLowerCase();
    return !f || e.path.toLowerCase().includes(f) || e.title.toLowerCase().includes(f) || e.desc.toLowerCase().includes(f);
  });

  return (
    <div className="mx-auto max-w-4xl px-4 py-12">
      <Seo title={`${t("dc.title")} — VEBOX`} desc={t("dc.desc")} path="/docs" />

      <div className="mb-8">
        <h1 className="text-3xl tracking-tight text-fg">{t("dc.title")}</h1>
        <p className="mt-3 max-w-xl text-sm leading-relaxed text-mut">{t("dc.desc")}</p>
      </div>

      <div className="mb-6 border border-line bg-panel p-5">
        <Micro className="mb-3">{t("dc.auth")}</Micro>
        <p className="text-xs leading-relaxed text-mut">{t("dc.auth.desc")}</p>
        <div className="mt-3 flex flex-wrap gap-2">
          <Badge tone="fg">X-API-Key</Badge>
          <Badge tone="fg">Authorization: Bearer</Badge>
          <Badge tone="fg">GitHub OAuth</Badge>
        </div>
      </div>

      <div className="mb-6 border border-line bg-panel p-5">
        <Micro className="mb-3">{t("dc.formats")}</Micro>
        <p className="text-xs leading-relaxed text-mut">{t("dc.formats.desc")}</p>
      </div>

      <div className="mb-8 border border-line bg-panel p-5">
        <Micro className="mb-3">{t("dc.delivery")}</Micro>
        <p className="text-xs leading-relaxed text-mut">{t("dc.delivery.desc")}</p>
      </div>

      <input
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        placeholder={t("common.search")}
        className="mb-4 w-full border border-line bg-panel px-3.5 py-3 text-sm text-fg placeholder:text-dim focus:border-line2 focus:outline-none"
      />

      <Micro className="mb-3">{t("dc.endpoints")}</Micro>
      <div className="space-y-3">
        {rows.map((e) => (
          <div key={e.m + e.path} className="border border-line bg-panel">
            <div className="flex flex-wrap items-center gap-3 border-b border-line px-4 py-3">
              <span className={`micro w-14 ${METHOD_COLOR[e.m]}`}>{e.m}</span>
              <code className="text-xs text-fg">{e.path}</code>
              <span className="micro ml-auto text-dim">{e.auth}</span>
            </div>
            <div className="space-y-2 px-4 py-3">
              <div className="text-xs text-fg">{e.title}</div>
              <div className="text-xs leading-relaxed text-mut">{e.desc}</div>
              <CodeLine code={e.sample} label={t("dc.try")} />
            </div>
          </div>
        ))}
        {rows.length === 0 && <div className="border border-line bg-panel px-4 py-6 text-center text-xs text-dim">{t("dc.none")}</div>}
      </div>

      <div className="mt-8 text-center">
        <a href={apiUrl("/openapi.json")} className="micro border border-line px-4 py-2.5 text-mut hover:border-line2 hover:text-fg">
          {t("dc.openapi")} <ExternalLink size={12} className="inline-block" aria-hidden />
        </a>
      </div>
    </div>
  );
}
