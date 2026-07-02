import React from 'react';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Cell } from 'recharts';

interface BookingsChartProps {
  data: {
    date: string;
    count: number;
  }[];
  isError?: boolean;
}

export const BookingsChart: React.FC<BookingsChartProps> = ({ data, isError }) => {
  if (isError) {
    return (
      <div className="bg-white p-6 rounded-3xl border border-border-main shadow-sm w-full h-[350px] flex flex-col items-center justify-center">
        <p className="text-red-500 font-medium text-center">Gagal memuat tren booking</p>
      </div>
    );
  }

  if (!data || data.length === 0) {
    return (
      <div className="bg-white p-6 rounded-3xl border border-border-main shadow-sm w-full h-[350px] flex flex-col items-center justify-center">
        <p className="text-text-muted font-medium text-center">Belum ada booking valid pada periode ini</p>
      </div>
    );
  }

  // Format date for display (e.g. "12 Nov")
  const formattedData = data.map(item => {
    const d = new Date(item.date);
    const months = ['Jan', 'Feb', 'Mar', 'Apr', 'Mei', 'Jun', 'Jul', 'Ags', 'Sep', 'Okt', 'Nov', 'Des'];
    return {
      day: `${d.getDate()} ${months[d.getMonth()]}`,
      bookings: item.count,
    };
  });

  return (
    <div className="bg-white p-6 rounded-3xl border border-border-main shadow-sm w-full h-[400px] flex flex-col">
      <div className="mb-6">
        <h3 className="text-lg font-extrabold text-text-main">Tren Jadwal Booking</h3>
      </div>
      <div className="flex-1 w-full">
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={formattedData} margin={{ top: 0, right: 0, left: -20, bottom: 0 }}>
            <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="#E2E8F0" />
            <XAxis 
              dataKey="day" 
              axisLine={false} 
              tickLine={false} 
              tick={{ fill: '#64748B', fontSize: 12, fontWeight: 600 }} 
              dy={10}
            />
            <YAxis 
              axisLine={false} 
              tickLine={false} 
              tick={{ fill: '#64748B', fontSize: 12, fontWeight: 600 }} 
            />
            <Tooltip 
              cursor={{ fill: '#F0FDFA' }}
              contentStyle={{ borderRadius: '12px', border: 'none', boxShadow: '0 10px 25px -3px rgba(15, 23, 42, 0.08)', fontWeight: 'bold' }}
              labelStyle={{ color: '#0F172A', marginBottom: '4px' }}
            />
            <Bar dataKey="bookings" radius={[6, 6, 6, 6]} maxBarSize={48}>
              {formattedData.map((_entry, index) => (
                <Cell key={`cell-${index}`} fill={index === formattedData.length - 1 ? '#0D9488' : '#99F6E4'} /> 
              ))}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
};
