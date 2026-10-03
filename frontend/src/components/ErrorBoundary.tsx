import { Component, type ErrorInfo, type ReactNode } from "react";
import { AlertTriangle, Home, RotateCw } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";

// ErrorFallback is a function component so it can use i18n; the boundary itself
// must be a class component to implement componentDidCatch.
function ErrorFallback({ error, onReset }: { error: Error; onReset: () => void }) {
  const { t } = useI18n();
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-background p-6 text-center">
      <div className="flex h-12 w-12 items-center justify-center rounded-full bg-destructive/10">
        <AlertTriangle className="h-6 w-6 text-destructive" />
      </div>
      <div>
        <h1 className="text-lg font-semibold">{t("error.title")}</h1>
        <p className="mt-1 max-w-md text-sm text-muted-foreground">{t("error.description")}</p>
      </div>
      {error.message && (
        <pre className="max-w-lg overflow-auto rounded-md bg-muted px-3 py-2 text-left text-xs text-muted-foreground">
          {error.message}
        </pre>
      )}
      <div className="flex flex-wrap justify-center gap-2">
        <Button onClick={onReset}>
          <RotateCw className="h-4 w-4" /> {t("error.retry")}
        </Button>
        <Button variant="outline" onClick={() => (window.location.href = "/")}>
          <Home className="h-4 w-4" /> {t("error.home")}
        </Button>
      </div>
    </div>
  );
}

interface Props {
  children: ReactNode;
}
interface State {
  error: Error | null;
}

// ErrorBoundary prevents a rendering exception in any page from blanking the
// whole SPA; it shows a recoverable fallback instead.
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // Keep a breadcrumb for debugging; a real deployment could ship this to
    // the audit/telemetry sink.
    console.error("Unhandled UI error:", error, info.componentStack);
  }

  private reset = () => this.setState({ error: null });

  render() {
    if (this.state.error) {
      return <ErrorFallback error={this.state.error} onReset={this.reset} />;
    }
    return this.props.children;
  }
}
