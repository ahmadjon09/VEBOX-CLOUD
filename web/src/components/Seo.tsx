
import { Helmet } from "react-helmet-async";

const SITE = "https://vebox.uz";

export function Seo({
  title,
  desc,
  path = "/",
  image = "/og.png",
  canonical,
}: {
  title: string;
  desc?: string;
  path?: string;
  image?: string;
  canonical?: string;
}) {
  const full = title;
  const url = canonical || `${SITE}${path}`;
  return (
    <Helmet>
      <html lang="uz" />
      <title>{full}</title>
      <link rel="canonical" href={url} />
      {desc && <meta name="description" content={desc} />}
      <meta property="og:title" content={full} />
      {desc && <meta property="og:description" content={desc} />}
      <meta property="og:url" content={url} />
      <meta property="og:image" content={`${SITE}${image}`} />
      <meta property="og:type" content="website" />
      <meta property="og:site_name" content="VEBOX" />
      <meta name="twitter:card" content="summary_large_image" />
      <meta name="twitter:title" content={full} />
      {desc && <meta name="twitter:description" content={desc} />}
    </Helmet>
  );
}

export function SchemaOrg() {
  return (
    <script type="application/ld+json">
      {JSON.stringify({
        "@context": "https://schema.org",
        "@type": "WebApplication",
        name: "VEBOX",
        url: SITE,
        applicationCategory: "UtilityApplication",
        operatingSystem: "Web",
        description:
          "Shaxsiy media saqlash va yetkazib berish platformasi: rasm va M4A/MP3 audio, CDN yetkazish, imzolangan havolalar.",
      })}
    </script>
  );
}
