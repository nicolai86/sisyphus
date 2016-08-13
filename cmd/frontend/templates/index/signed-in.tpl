<html>
  <body>
    <div>
      <h1>private</h1>
      <div>
        <h2>repositories</h2>
        <ul>
          {{ range .Repositories }}
          <li>
            {{ .Name }}
            <form action="/toggle" method="POST">
              <input
                id="service"
                name="service"
                type="hidden"
                value="greenkeep"
              >
              <input
                id="repository_id"
                name="repository_id"
                type="hidden"
                value="{{ .ID }}"
              >
              <input
                id="repository_git_url"
                name="repository_git_url"
                type="hidden"
                value="{{ .GitURL }}"
              >
              <input
                id="repository_name"
                name="repository_name"
                type="hidden"
                value="{{ .FullName }}"
              >
              <input
                  id="action"
                  name="action"
                  value="{{ if enabled .FullName "greenkeep" }}disable{{ else }}enable{{ end }}"
                  type="hidden"
              >
              <button>
              {{ if enabled .FullName "greenkeep" }}
                  Disable
                {{ else }}
                  Enable
                {{ end }}
              </button>
            </form>
          </li>
          {{ end }}
        </ul>
      </div>
    </div>

    <div>
      {{ range .Organizations }}
      <div>
        <h1>{{ .Login }}</h1>
        <div>
          <h2>repositories</h2>
          <ul>
            {{ range .Repositories }}
            <li>
              {{ .Name }}
              <form action="/toggle" method="POST">
                <input
                  id="service"
                  type="hidden"
                  name="service"
                  value="greenkeep"
                >
                <input
                  id="repository_id"
                  name="repository_id"
                  value="{{ .ID }}"
                  type="hidden"
                >
                <input
                  id="repository_git_url"
                  name="repository_git_url"
                  value="{{ .GitURL }}"
                  type="hidden"
                >
                <input
                  id="repository_name"
                  name="repository_name"
                  value="{{ .FullName }}"
                  type="hidden"
                >
                <input
                  id="action"
                  name="action"
                  value="{{ if enabled .FullName "greenkeep" }}disable{{ else }}enable{{ end }}"
                  type="hidden"
                >
                <button>
                {{ if enabled .FullName "greenkeep" }}
                  Disable
                {{ else }}
                  Enable
                {{ end }}
                </button>
              </form>
            </li>
            {{ end }}
          </ul>
        </div>
      </div>
      {{ end }}
    </div>

    <div>
      <a href="/logout">Logout</a>
    </div>
  </body>
</html>
