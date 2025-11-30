import { Card, CardContent } from "@/components/ui/card";
import { ResponsivePageContainer } from "@/components/ui/responsive-container";
import { useResponsive } from "@/hooks/useResponsive";
import { api } from "@/lib/api";
import { marked } from "marked";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

export function HomePage() {
  const [home, setHome] = useState(""); // URL or rendered HTML
  const [loaded, setLoaded] = useState(false);
  const { isMobile } = useResponsive();
  const { t } = useTranslation();

  const loadHome = async () => {
    try {
      // Load cached rendered HTML first for faster first paint
      const cachedHtml = localStorage.getItem("home_page_content_html");
      const cachedRaw = localStorage.getItem("home_page_content");
      if (cachedHtml) {
        setHome(cachedHtml);
      } else if (cachedRaw) {
        // Backward compatibility: previously cached raw markdown
        const rendered = cachedRaw.startsWith("https://")
          ? cachedRaw
          : marked.parse(cachedRaw);
        setHome(rendered as string);
        localStorage.setItem("home_page_content_html", rendered as string);
      }

      // Fetch latest from backend
      const res = await api.get("/api/home_page_content");
      const { success, data } = res.data;
      if (success && typeof data === "string") {
        // If data is a URL, use it directly; otherwise render Markdown to HTML
        const rendered = data.startsWith("https://")
          ? data
          : (marked.parse(data) as string);
        setHome(rendered);
        // Cache both raw and rendered for future loads
        localStorage.setItem("home_page_content", data);
        localStorage.setItem("home_page_content_html", rendered);
      }
    } catch (err) {
      // Keep any cached content; fall back to default UI below if none
      console.error("Error loading home page content:", err);
    } finally {
      setLoaded(true);
    }
  };

  useEffect(() => {
    loadHome();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // If home is a URL, render as iframe to allow embedding an external page
  if (home.startsWith("https://")) {
    return (
      <iframe
        src={home}
        className="w-full h-screen border-0"
        title={t("home.iframe_title")}
      />
    );
  }

  // If custom content exists (HTML/Markdown), render it; Markdown is pre-rendered to HTML
  if (loaded && home) {
    return (
      <ResponsivePageContainer>
        <Card>
          <CardContent className={isMobile ? "p-4" : "p-6"}>
            <div
              className="prose prose-lg max-w-full break-words dark:prose-invert prose-headings:font-semibold prose-headings:tracking-tight prose-headings:break-words prose-headings:whitespace-pre-wrap prose-a:text-primary prose-a:break-words hover:prose-a:underline prose-a:break-all prose-pre:bg-muted/60 prose-pre:overflow-x-auto prose-code:before:content-[''] prose-code:after:content-['']"
              dangerouslySetInnerHTML={{ __html: home }}
            />
          </CardContent>
        </Card>
      </ResponsivePageContainer>
    );
  }

  // Minimal empty state when no custom home content is configured
  return (
    <ResponsivePageContainer>
      <div className={isMobile ? "py-8" : "py-16"} data-testid="home-empty" />
    </ResponsivePageContainer>
  );
}
