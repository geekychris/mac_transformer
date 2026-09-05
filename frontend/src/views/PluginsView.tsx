import { PluginView } from '../api'

interface Props {
  plugins: PluginView[]
  onReload: () => void
}

export function PluginsView({ plugins, onReload }: Props) {
  return (
    <div className="plugins">
      <div className="toolbar">
        <button className="btn" onClick={onReload}>Re-probe availability</button>
      </div>
      <table className="plugin-tbl">
        <thead>
          <tr>
            <th>Name</th>
            <th>Available</th>
            <th>Capabilities</th>
            <th>Notes</th>
          </tr>
        </thead>
        <tbody>
          {plugins.map((p) => (
            <tr key={p.name} className={p.available ? '' : 'row-dim'}>
              <td>
                <div className="pname">{p.name}</div>
                <div className="muted">{p.description}</div>
              </td>
              <td>{p.available ? '✓' : '—'}</td>
              <td>
                <ul className="caps">
                  {p.capabilities.map((c, i) => (
                    <li key={i} className="mono">
                      {c.from.join('|')} → {c.to.join('|')} @{c.priority}
                    </li>
                  ))}
                </ul>
              </td>
              <td className="muted">{p.reason}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
