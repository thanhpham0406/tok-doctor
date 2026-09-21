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

type LoadState =
  | { state: "loading" }
  | { state: "ready"; result: AnalysisResult }
  | { state: "unavailable" };

function App() {
  return (
    <main>
      <h1>TokDoctor</h1>
      <ResultView />
    </main>
  );
}

function ResultView() {
  const [load, setLoad] = useState<LoadState>({ state: "loading" });

  useEffect(() => {
    fetch("/api/result")
      .then((response) => {
        if (!response.ok) {
          throw new Error(`result request failed with status ${response.status}`);
        }
        return response.json();
      })
      .then((data: AnalysisResult) => setLoad({ state: "ready", result: data }))
      .catch(() => setLoad({ state: "unavailable" }));
  }, []);

  if (load.state === "loading") {
    return (
      <section>
        <p>Waiting for the local analysis result.</p>
      </section>
    );
  }

  if (load.state === "unavailable") {
    return (
      <section>
        <p>
          Analysis result unavailable. The UI could not read /api/result from the
          local TokDoctor server and has no data to show.
        </p>
      </section>
    );
  }

  const { result } = load;
  return (
    <section>
      <p>{result.summary.message}</p>
      <dl>
        <dt>Source</dt>
        <dd>{displaySource(result.source)}</dd>
        <dt>Status</dt>
        <dd>{result.summary.status}</dd>
        <dt>Findings</dt>
        <dd>{result.findings.length}</dd>
      </dl>
    </section>
  );
}

function displaySource(source: string) {
  return source === "" ? "unknown" : source;
}

render(<App />, document.querySelector("#app")!);
