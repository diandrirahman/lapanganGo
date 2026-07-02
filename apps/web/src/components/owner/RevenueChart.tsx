import React from 'react';
import { PieChart, Pie, Cell, ResponsiveContainer, Tooltip, Legend } from 'recharts';

interface RevenueChartProps {
  data: {
    name: string;
    value: number;
  }[];
  isError?: boolean;
  subtitle?: string;
}

const COLORS = ['#0D9488', '#2DD4BF', '#99F6E4', '#5EEAD4', '#CCFBF1'];

export const RevenueChart: React.FC<RevenueChartProps> = ({ data, isError, subtitle }) => {
  if (isError) {
    return (
      <div className="bg-white p-6 rounded-3xl border border-border-main shadow-sm w-full h-[350px] flex flex-col items-center justify-center">
        <p className="text-red-500 font-medium text-center">Gagal memuat tren pendapatan</p>
      </div>
    );
  }

  const totalValue = data ? data.reduce((a, b) => a + b.value, 0) : 0;

  if (!data || data.length === 0 || totalValue <= 0) {
    return (
      <div className="bg-white p-6 rounded-3xl border border-border-main shadow-sm w-full h-[350px] flex flex-col items-center justify-center">
        <p className="text-text-muted font-medium text-center">Belum ada pendapatan pada periode ini</p>
      </div>
    );
  }

  const positiveData = data.filter(item => item.value > 0);

  return (
    <div className="bg-white p-6 rounded-3xl border border-border-main shadow-sm w-full h-[400px] flex flex-col">
      <div className="mb-2">
        <h3 className="text-lg font-extrabold text-text-main">Pendapatan Booking per Venue</h3>
        {subtitle && <p className="text-xs font-medium text-text-muted mt-1">{subtitle}</p>}
      </div>
      <div className="flex-1 w-full relative">
        <ResponsiveContainer width="100%" height="100%">
          <PieChart>
            <Pie
              data={positiveData}
              cx="50%"
              cy="45%"
              innerRadius={65}
              outerRadius={90}
              paddingAngle={5}
              dataKey="value"
              stroke="none"
            >
              {positiveData.map((_entry, index) => (
                <Cell key={`cell-${index}`} fill={COLORS[index % COLORS.length]} />
              ))}
            </Pie>
            <Tooltip 
              formatter={(value: any) => new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(Number(value))}
              contentStyle={{ borderRadius: '12px', border: 'none', boxShadow: '0 10px 25px -3px rgba(15, 23, 42, 0.08)', fontWeight: 'bold' }}
              itemStyle={{ color: '#0F172A' }}
            />
            <Legend 
              verticalAlign="bottom" 
              height={72} 
              iconType="circle" 
              formatter={(value) => <span className="text-xs font-bold text-text-main">{value}</span>}
            />
          </PieChart>
        </ResponsiveContainer>
        {/* Center Text overlay */}
        <div className="absolute inset-0 flex flex-col items-center justify-center pointer-events-none mt-[-72px]">
          <span className="text-[10px] font-bold text-text-muted uppercase tracking-wider">Total</span>
          <span className="text-base font-extrabold text-text-main">
            {new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(totalValue)}
          </span>
        </div>
      </div>
    </div>
  );
};
