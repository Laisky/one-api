import { Button } from "@/components/ui/button";
import { Home } from "lucide-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useNavigate } from "react-router-dom";

export function NotFoundPage() {
  const navigate = useNavigate();
  const [seconds, setSeconds] = useState(5);
  const { t } = useTranslation();

  useEffect(() => {
    const tick = setInterval(() => setSeconds((s) => Math.max(0, s - 1)), 1000);
    const timer = setTimeout(() => navigate("/", { replace: true }), 5000);
    return () => {
      clearInterval(tick);
      clearTimeout(timer);
    };
  }, [navigate]);

  return (
    <div className="flex flex-col items-center justify-center text-center py-16 gap-6">
      <div>
        <h1 className="text-4xl font-bold">404</h1>
        <p className="text-muted-foreground mt-2">{t("notFound.message")}</p>
      </div>

      <p className="text-sm text-muted-foreground">
        {t("notFound.redirecting", { seconds })}
      </p>

      <div className="flex items-center gap-3">
        <Button asChild>
          <Link to="/">
            <Home className="mr-2 h-4 w-4" /> {t("notFound.go_home")}
          </Link>
        </Button>
        <Button variant="outline" onClick={() => navigate(-1)}>
          {t("common.back")}
        </Button>
      </div>
    </div>
  );
}

export default NotFoundPage;
