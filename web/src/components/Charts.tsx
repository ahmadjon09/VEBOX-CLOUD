
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  BarChart,
  Bar,
  PieChart,
  Pie,
  Cell,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
} from "recharts";

const AXIS = {
  stroke: "rgba(255,255,255,0.09)",
  tick: { fill: "#8b8b8b", fontSize: 10, fontFamily: "inherit" },
  tickLine: false as const,
  axisLine: { stroke: "rgba(255,255,255,0.09)" },
};

const COLORS = ["#e9e9e9", "#3ddc84", "#e8b93d", "#ef5f5f", "#8b8b8b"];

export function ChartFrame({ children, label }: { children: React.ReactNode; label?: string }) {
  return (
    <div className="border border-line bg-panel">
      {label && <div className="micro border-b border-line px-4 py-3 text-mut">{label}</div>}
      <div className="h-64 w-full p-3">{children}</div>
    </div>
  );
}

export function TrendChart({
  data,
}: {
  data: { date: string; view: number; upload: number; bytes: number }[];
}) {
  const rows = data.map((d) => ({ ...d, day: d.date.slice(5) }));
  return (
    <ResponsiveContainer width="100%" height="100%">
      <AreaChart data={rows} margin={{ top: 8, right: 8, bottom: 0, left: -18 }}>
        <defs>
          <linearGradient id="gView" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#e9e9e9" stopOpacity={0.25} />
            <stop offset="100%" stopColor="#e9e9e9" stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="1 6" vertical={false} />
        <XAxis dataKey="day" {...AXIS} minTickGap={24} />
        <YAxis {...AXIS} allowDecimals={false} />
        <Tooltip cursor={{ stroke: "rgba(255,255,255,0.2)" }} />
        <Area type="monotone" dataKey="view" name="Views" stroke="#e9e9e9" strokeWidth={1.5} fill="url(#gView)" />
        <Area type="monotone" dataKey="upload" name="Uploads" stroke="#3ddc84" strokeWidth={1.2} fill="transparent" />
      </AreaChart>
    </ResponsiveContainer>
  );
}

export function BytesChart({ data }: { data: { date: string; bytes: number }[] }) {
  const rows = data.map((d) => ({ day: d.date.slice(5), bytes: d.bytes }));
  return (
    <ResponsiveContainer width="100%" height="100%">
      <BarChart data={rows} margin={{ top: 8, right: 8, bottom: 0, left: -18 }}>
        <CartesianGrid strokeDasharray="1 6" vertical={false} />
        <XAxis dataKey="day" {...AXIS} minTickGap={24} />
        <YAxis {...AXIS} tickFormatter={(v: number) => (v >= 1e6 ? `${(v / 1e6).toFixed(0)}M` : v >= 1e3 ? `${(v / 1e3).toFixed(0)}K` : String(v))} />
        <Tooltip cursor={{ fill: "rgba(255,255,255,0.05)" }} />
        <Bar dataKey="bytes" name="Bytes" fill="#e8b93d" fillOpacity={0.8} maxBarSize={18} />
      </BarChart>
    </ResponsiveContainer>
  );
}

export function ActionPie({ data }: { data: { action: string; count: number }[] }) {
  return (
    <ResponsiveContainer width="100%" height="100%">
      <PieChart>
        <Pie
          data={data}
          dataKey="count"
          nameKey="action"
          innerRadius="55%"
          outerRadius="85%"
          paddingAngle={3}
          stroke="none"
        >
          {data.map((_, i) => (
            <Cell key={i} fill={COLORS[i % COLORS.length]} />
          ))}
        </Pie>
        <Tooltip />
      </PieChart>
    </ResponsiveContainer>
  );
}
