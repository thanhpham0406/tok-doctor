import { render } from "preact";
import { useEffect, useState } from "preact/hooks";
import "./style.css";

type AnalysisResult = {
  source: string;
  summary: {
    status: string;
    message: string;
  };
  findings: Array<unknown>;
};

const fallback: AnalysisResult = {
  source: "codex",
  summary: {
    status: "ready",
    message: "TokDoctor is initialized. No diagnostic rules are enabled in this scaffold.",
  },
  findings: [],
};

function App() {
  return (
    <main>
      <h1>TokDoctor</h1>
      <ResultView />
    </main>
  );
}

function ResultView() {
  const [result, setResult] = useState<AnalysisResult>(fallback);

  useEffect(() => {
    fetch("/api/result")
      .then((response) => response.json())
      .then((data: AnalysisResult) => setResult(data))
      .catch(() => setResult(fallback));
  }, []);

  return (
    <section>
      <p>{result.summary.message}</p>
      <dl>
        <dt>Source</dt>
        <dd>{result.source}</dd>
        <dt>Status</dt>
        <dd>{result.summary.status}</dd>
        <dt>Findings</dt>
        <dd>{result.findings.length}</dd>
      </dl>
    </section>
  );
}

render(<App />, document.querySelector("#app")!);
