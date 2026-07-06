import React, { useEffect, useState, useCallback } from 'react';
import { PageShell } from '../../components/layout/PageShell';
import { useSearchParams } from 'react-router-dom';
import { useAuth } from '../../contexts/AuthContext';
import { fetchOwnerFinanceSummary, fetchOwnerVenues, fetchTransactions, createTransaction } from '../../lib/api';
import { LoadingState } from '../../components/feedback/LoadingState';
import { ErrorState } from '../../components/feedback/ErrorState';
import { formatRupiah, getLocalTodayDateString } from '../../lib/utils';
import { Wallet, TrendingUp, TrendingDown, Activity, Receipt, LayoutDashboard, ListPlus, X, AlertCircle, Undo2 } from 'lucide-react';
import type { FinanceSummaryResult, TransactionListResponse } from '../../types/finance';
import type { Venue } from '../../types/venue';
import { RevenueChart } from '../../components/owner/RevenueChart';

export const OwnerFinancePage: React.FC = () => {
  const { token } = useAuth();
  const [summaryData, setSummaryData] = useState<FinanceSummaryResult | null>(null);
  const [transactionsData, setTransactionsData] = useState<TransactionListResponse | null>(null);
  const [venues, setVenues] = useState<Venue[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [searchParams, setSearchParams] = useSearchParams();
  
  // Filters
  const [venueId, setVenueId] = useState(searchParams.get('venue_id') || '');
  const [startDate, setStartDate] = useState('');
  const [endDate, setEndDate] = useState('');
  const [activeTab, setActiveTab] = useState<'summary' | 'transactions'>('summary');

  const [isModalOpen, setIsModalOpen] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [modalForm, setModalForm] = useState({
    type: 'INCOME' as 'INCOME' | 'EXPENSE',
    categorySelect: 'SEWA_ALAT',
    customCategory: '',
    amount: '',
    transaction_date: getLocalTodayDateString(),
    description: '',
    venue_id: '',
  });

  const loadData = useCallback(async () => {
    if (!token) return;
    setIsLoading(true);
    try {
      if (venues.length === 0) {
        const v = await fetchOwnerVenues(token);
        setVenues(v);
      }

      if (activeTab === 'summary') {
        const summary = await fetchOwnerFinanceSummary(token, {
          venue_id: venueId || undefined,
          start_date: startDate || undefined,
          end_date: endDate || undefined,
        });
        setSummaryData(summary);
      } else {
        const txs = await fetchTransactions(token, {
          venue_id: venueId || undefined,
          start_date: startDate || undefined,
          end_date: endDate || undefined,
          page: 1,
          limit: 50, // simple pagination for now
        });
        setTransactionsData(txs);
      }
      
      setError(null);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setIsLoading(false);
    }
  }, [token, venueId, startDate, endDate, activeTab, venues.length]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const handleReset = () => {
    setVenueId('');
    setStartDate('');
    setEndDate('');
    const newParams = new URLSearchParams(searchParams);
    newParams.delete('venue_id');
    newParams.delete('start_date');
    newParams.delete('end_date');
    setSearchParams(newParams);
  };

  const handleCreateTransaction = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token) return;
    setIsSubmitting(true);

    const finalCategory = modalForm.categorySelect === 'LAINNYA' 
      ? modalForm.customCategory.trim() 
      : modalForm.categorySelect;

    if (modalForm.categorySelect === 'LAINNYA' && !finalCategory) {
      alert('Kategori custom tidak boleh kosong');
      setIsSubmitting(false);
      return;
    }

    try {
      await createTransaction(token, {
        type: modalForm.type,
        category: finalCategory,
        amount: parseFloat(modalForm.amount),
        transaction_date: modalForm.transaction_date,
        description: modalForm.description || undefined,
        venue_id: modalForm.venue_id || undefined,
      });
      setIsModalOpen(false);
      setModalForm({
        type: 'INCOME',
        categorySelect: 'SEWA_ALAT',
        customCategory: '',
        amount: '',
        transaction_date: getLocalTodayDateString(),
        description: '',
        venue_id: '',
      });
      loadData();
    } catch (err: any) {
      alert(err.message || 'Gagal membuat transaksi');
    } finally {
      setIsSubmitting(false);
    }
  };

  const metricCards = summaryData ? [
    {
      label: 'Total Pemasukan',
      subtitle: 'Booking + manual',
      value: formatRupiah(summaryData.total_income),
      icon: TrendingUp,
      tone: 'bg-green-50 text-green-600',
      group: 'income',
    },
    {
      label: 'Pendapatan Booking',
      subtitle: 'Dari transaksi booking',
      value: formatRupiah(summaryData.realized_booking_revenue),
      icon: Activity,
      tone: 'bg-blue-50 text-blue-600',
      group: 'income',
    },
    {
      label: 'Pemasukan Manual',
      subtitle: 'Sponsor, sewa alat, lainnya',
      value: formatRupiah(summaryData.manual_income),
      icon: Receipt,
      tone: 'bg-indigo-50 text-indigo-600',
      group: 'income',
    },
    {
      label: 'Total Pengeluaran',
      subtitle: 'Refund + biaya manual',
      value: formatRupiah(summaryData.total_expense),
      icon: TrendingDown,
      tone: 'bg-red-50 text-red-600',
      group: 'profit',
    },
    {
      label: 'Refund',
      subtitle: 'Pengembalian booking',
      value: formatRupiah(summaryData.refund_expense),
      icon: Undo2,
      tone: 'bg-rose-50 text-rose-600',
      group: 'profit',
    },
    {
      label: 'Laba Bersih',
      subtitle: 'Total pemasukan - pengeluaran',
      value: formatRupiah(summaryData.net_profit),
      icon: Wallet,
      tone: 'bg-primary-50 text-primary',
      group: 'profit',
    },
  ] : [];

  return (
    <PageShell>
      <div className="pt-28 md:pt-24 pb-40 max-w-6xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="mb-8 max-w-3xl">
          <h1 className="text-3xl md:text-4xl font-extrabold text-text-main mb-2">Keuangan & Ledger</h1>
          <p className="text-text-muted text-sm sm:text-base">Pantau arus kas dan kelola transaksi venue Anda</p>
        </div>

        {/* Filters */}
        <div className="bg-white p-4 rounded-2xl border border-border-main mb-6 shadow-sm grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-[minmax(220px,1fr)_minmax(180px,220px)_minmax(180px,220px)_minmax(140px,180px)] gap-4 lg:items-end">
          <div>
            <label className="block text-xs font-bold text-text-muted mb-1 uppercase tracking-wider">Venue</label>
            <select
              className="w-full h-10 px-4 rounded-xl border border-border-main text-sm focus:ring-2 focus:ring-primary outline-none bg-white"
              value={venueId}
              onChange={(e) => {
                const val = e.target.value;
                setVenueId(val);
                const newParams = new URLSearchParams(searchParams);
                if (val) {
                  newParams.set('venue_id', val);
                } else {
                  newParams.delete('venue_id');
                }
                setSearchParams(newParams);
              }}
            >
              <option value="">Semua Venue</option>
              {venues.map(v => (
                <option key={v.id} value={v.id}>{v.name}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs font-bold text-text-muted mb-1 uppercase tracking-wider">Dari Tanggal</label>
            <input
              type="date"
              className="w-full h-10 px-4 rounded-xl border border-border-main text-sm focus:ring-2 focus:ring-primary outline-none"
              value={startDate}
              onChange={(e) => setStartDate(e.target.value)}
            />
          </div>
          <div>
            <label className="block text-xs font-bold text-text-muted mb-1 uppercase tracking-wider">Sampai Tanggal</label>
            <input
              type="date"
              className="w-full h-10 px-4 rounded-xl border border-border-main text-sm focus:ring-2 focus:ring-primary outline-none"
              value={endDate}
              onChange={(e) => setEndDate(e.target.value)}
            />
          </div>
          <div className="sm:col-span-2 lg:col-span-1">
            <button 
              onClick={handleReset}
              className="w-full h-10 bg-gray-100 hover:bg-gray-200 text-text-main font-bold rounded-xl text-sm transition-colors"
            >
              Reset Filter
            </button>
          </div>
        </div>

        {/* Tabs */}
        <div className="flex space-x-1 bg-gray-100 p-1 rounded-2xl mb-8 max-w-sm">
          <button
            className={`flex-1 py-2.5 px-4 rounded-xl text-sm font-bold transition-all ${
              activeTab === 'summary' 
                ? 'bg-white text-primary shadow-sm' 
                : 'text-text-muted hover:text-text-main'
            }`}
            onClick={() => setActiveTab('summary')}
          >
            <div className="flex items-center justify-center space-x-2">
              <LayoutDashboard size={16} />
              <span>Ringkasan</span>
            </div>
          </button>
          <button
            className={`flex-1 py-2.5 px-4 rounded-xl text-sm font-bold transition-all ${
              activeTab === 'transactions' 
                ? 'bg-white text-primary shadow-sm' 
                : 'text-text-muted hover:text-text-main'
            }`}
            onClick={() => setActiveTab('transactions')}
          >
            <div className="flex items-center justify-center space-x-2">
              <Receipt size={16} />
              <span>Transaksi</span>
            </div>
          </button>
        </div>

        {error && (
          <div className="mb-8">
            <ErrorState 
              title="Gagal memuat data keuangan" 
              message={error} 
              onRetry={loadData} 
            />
          </div>
        )}

        {isLoading ? (
          <div className="py-20">
            <LoadingState message="Menghitung arus kas..." />
          </div>
        ) : !error ? (
          activeTab === 'summary' ? (
            summaryData && (
              <div className="space-y-8 animate-fade-in">
                {/* Metric Cards - Income Group */}
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 sm:gap-4 mb-3 sm:mb-4">
                  {metricCards.filter(c => c.group === 'income').map((card, idx) => (
                    <div key={idx} className="bg-white p-4 sm:p-5 rounded-2xl border border-border-main shadow-sm flex items-center gap-3 sm:gap-4">
                      <div className={`p-3 sm:p-4 rounded-xl shrink-0 ${card.tone}`}>
                        <card.icon className="w-6 h-6 sm:w-7 sm:h-7" />
                      </div>
                      <div className="min-w-0 flex-1">
                        <p className="text-xs sm:text-sm font-bold text-text-muted uppercase tracking-wider mb-0.5">{card.label}</p>
                        <p className="text-lg sm:text-xl lg:text-2xl font-extrabold text-text-main">{card.value}</p>
                        {card.subtitle && <p className="text-[10px] sm:text-xs text-text-muted mt-0.5 font-medium">{card.subtitle}</p>}
                      </div>
                    </div>
                  ))}
                </div>
                {/* Metric Cards - Profit Group */}
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 sm:gap-4">
                  {metricCards.filter(c => c.group === 'profit').map((card, idx) => (
                    <div key={idx} className="bg-white p-4 sm:p-5 rounded-2xl border border-border-main shadow-sm flex items-center gap-3 sm:gap-4">
                      <div className={`p-3 sm:p-4 rounded-xl shrink-0 ${card.tone}`}>
                        <card.icon className="w-6 h-6 sm:w-7 sm:h-7" />
                      </div>
                      <div className="min-w-0 flex-1">
                        <p className="text-xs sm:text-sm font-bold text-text-muted uppercase tracking-wider mb-0.5">{card.label}</p>
                        <p className="text-lg sm:text-xl lg:text-2xl font-extrabold text-text-main">{card.value}</p>
                        {card.subtitle && <p className="text-[10px] sm:text-xs text-text-muted mt-0.5 font-medium">{card.subtitle}</p>}
                      </div>
                    </div>
                  ))}
                </div>

                <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
                  {/* Revenue Breakdown Chart */}
                  <RevenueChart 
                    data={summaryData.venue_breakdown.map(v => ({ name: v.venue_name, value: v.realized_revenue }))} 
                    subtitle="Berdasarkan ledger booking. Pemasukan manual ditampilkan di ringkasan kas." 
                  />
                  
                  {/* Expense Breakdown List */}
                  <div className="bg-white p-6 rounded-3xl border border-border-main shadow-sm h-[350px] flex flex-col">
                    <div className="mb-4">
                      <h3 className="text-lg font-extrabold text-text-main">Pengeluaran per Kategori</h3>
                      <p className="text-sm font-medium text-text-muted">Rincian biaya operasional</p>
                    </div>
                    <div className="flex-1 overflow-y-auto pr-2 space-y-3 custom-scrollbar">
                      {summaryData.expense_by_category.length > 0 ? (
                        summaryData.expense_by_category.map((exp, idx) => (
                          <div key={idx} className="flex justify-between items-center p-3 hover:bg-gray-50 rounded-xl transition-colors border border-border-main/50">
                            <span className="font-bold text-text-main">{exp.category}</span>
                            <span className="font-extrabold text-red-600">{formatRupiah(exp.amount)}</span>
                          </div>
                        ))
                      ) : (
                        <div className="h-full flex items-center justify-center text-text-muted font-medium">
                          Belum ada pengeluaran
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              </div>
            )
          ) : (
            transactionsData && (
              <div className="space-y-6 animate-fade-in">
                <div className="flex justify-between items-center bg-white p-4 rounded-2xl border border-border-main shadow-sm">
                  <div>
                    <h3 className="text-lg font-extrabold text-text-main">Daftar Transaksi</h3>
                    <p className="text-sm text-text-muted font-medium">Total {transactionsData.total} transaksi</p>
                  </div>
                  <button 
                    onClick={() => setIsModalOpen(true)}
                    className="flex items-center space-x-2 bg-primary hover:bg-primary/90 text-white px-5 py-2.5 rounded-xl font-bold transition-all shadow-sm"
                  >
                    <ListPlus size={18} />
                    <span>Tambah</span>
                  </button>
                </div>

                <div className="bg-white rounded-3xl border border-border-main shadow-sm overflow-hidden">
                  <div className="overflow-x-auto">
                    <table className="w-full text-left border-collapse min-w-[800px]">
                      <thead>
                        <tr className="border-b border-border-main bg-gray-50/50">
                          <th className="py-4 px-6 text-xs font-bold text-text-muted uppercase tracking-wider">Tanggal</th>
                          <th className="py-4 px-6 text-xs font-bold text-text-muted uppercase tracking-wider">Jenis</th>
                          <th className="py-4 px-6 text-xs font-bold text-text-muted uppercase tracking-wider">Kategori/Deskripsi</th>
                          <th className="py-4 px-6 text-xs font-bold text-text-muted uppercase tracking-wider">Sumber</th>
                          <th className="py-4 px-6 text-xs font-bold text-text-muted uppercase tracking-wider text-right">Jumlah</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-border-main">
                        {transactionsData.transactions.length > 0 ? (
                          transactionsData.transactions.map((tx) => (
                            <tr key={tx.id} className="hover:bg-gray-50 transition-colors">
                              <td className="py-4 px-6">
                                <span className="text-sm font-bold text-text-main">{tx.transaction_date}</span>
                              </td>
                              <td className="py-4 px-6">
                                <span className={`inline-flex items-center px-2.5 py-1 rounded-lg text-xs font-bold ${
                                  tx.type === 'INCOME' ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'
                                }`}>
                                  {tx.type === 'INCOME' ? 'Pemasukan' : 'Pengeluaran'}
                                </span>
                              </td>
                              <td className="py-4 px-6">
                                <p className="text-sm font-bold text-text-main">{tx.category}</p>
                                {tx.description && <p className="text-xs text-text-muted mt-1 truncate max-w-[200px]">{tx.description}</p>}
                              </td>
                              <td className="py-4 px-6">
                                <span className={`inline-flex items-center px-2.5 py-1 rounded-lg text-xs font-bold ${
                                  tx.source === 'BOOKING' ? 'bg-teal-100 text-teal-700' :
                                  tx.source === 'MANUAL' ? 'bg-blue-100 text-blue-700' :
                                  tx.source === 'REFUND' ? 'bg-red-100 text-red-700' :
                                  'bg-gray-100 text-gray-700'
                                }`}>
                                  {tx.source === 'BOOKING' ? 'Booking' :
                                   tx.source === 'MANUAL' ? 'Manual' :
                                   tx.source === 'REFUND' ? 'Refund' : tx.source}
                                </span>
                              </td>
                              <td className="py-4 px-6 text-right">
                                <span className={`text-sm font-extrabold ${tx.type === 'INCOME' ? 'text-green-600' : 'text-red-600'}`}>
                                  {tx.type === 'INCOME' ? '+' : '-'}{formatRupiah(tx.amount)}
                                </span>
                              </td>
                            </tr>
                          ))
                        ) : (
                          <tr>
                            <td colSpan={5} className="py-8 text-center text-text-muted font-medium">
                              Belum ada transaksi pada periode ini
                            </td>
                          </tr>
                        )}
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>
            )
          )
        ) : null}
      </div>

      {/* Create Transaction Modal */}
      {isModalOpen && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4">
          <div className="bg-white rounded-2xl w-full max-w-md overflow-hidden shadow-xl flex flex-col max-h-[90vh]">
            <div className="flex justify-between items-center p-4 border-b border-border-main shrink-0">
              <h3 className="font-extrabold text-lg text-text-main">Tambah Transaksi Manual</h3>
              <button onClick={() => setIsModalOpen(false)} className="text-text-muted hover:text-text-main p-1">
                <X size={20} />
              </button>
            </div>
            
            <div className="p-4 overflow-y-auto custom-scrollbar flex-1">
              <form id="createTxForm" onSubmit={handleCreateTransaction} className="space-y-4">
                <div className="bg-blue-50 text-blue-800 p-3 rounded-xl text-sm font-medium flex items-start gap-2">
                  <AlertCircle className="w-5 h-5 shrink-0 mt-0.5 text-blue-600" />
                  <p>Transaksi manual tidak membuat booking dan tidak memblokir jadwal. Booking langsung/walk-in akan dikelola melalui fitur terpisah.</p>
                </div>
                <div>
                  <label className="block text-xs font-bold text-text-muted mb-1 uppercase tracking-wider">Jenis Transaksi</label>
                  <select 
                    required
                    value={modalForm.type}
                    onChange={(e) => {
                      const newType = e.target.value as 'INCOME' | 'EXPENSE';
                      setModalForm({
                        ...modalForm, 
                        type: newType,
                        categorySelect: newType === 'INCOME' ? 'SEWA_ALAT' : 'LISTRIK'
                      });
                    }}
                    className="w-full px-3 py-2 border border-border-main rounded-xl outline-none focus:border-primary text-sm"
                  >
                    <option value="INCOME">Pemasukan Manual</option>
                    <option value="EXPENSE">Pengeluaran Manual</option>
                  </select>
                </div>
                
                <div>
                  <label className="block text-xs font-bold text-text-muted mb-1 uppercase tracking-wider">Kategori</label>
                  <select
                    required
                    value={modalForm.categorySelect}
                    onChange={(e) => setModalForm({...modalForm, categorySelect: e.target.value})}
                    className="w-full px-3 py-2 border border-border-main rounded-xl outline-none focus:border-primary text-sm bg-white"
                  >
                    {modalForm.type === 'INCOME' ? (
                      <>
                        <option value="SEWA_ALAT">Sewa Alat</option>
                        <option value="SPONSOR">Sponsor</option>
                        <option value="PENJUALAN_MINUMAN">Penjualan Minuman</option>
                        <option value="LAINNYA">Lainnya</option>
                      </>
                    ) : (
                      <>
                        <option value="LISTRIK">Listrik</option>
                        <option value="AIR">Air</option>
                        <option value="KEBERSIHAN">Kebersihan</option>
                        <option value="MAINTENANCE">Maintenance</option>
                        <option value="GAJI">Gaji</option>
                        <option value="PERLENGKAPAN">Perlengkapan</option>
                        <option value="LAINNYA">Lainnya</option>
                      </>
                    )}
                  </select>
                </div>
                
                {modalForm.categorySelect === 'LAINNYA' && (
                  <div>
                    <label className="block text-xs font-bold text-text-muted mb-1 uppercase tracking-wider">Nama Kategori Custom</label>
                    <input 
                      type="text" 
                      required
                      placeholder="Masukkan nama kategori"
                      value={modalForm.customCategory}
                      onChange={(e) => setModalForm({...modalForm, customCategory: e.target.value.toUpperCase().replace(/\s+/g, '_')})}
                      className="w-full px-3 py-2 border border-border-main rounded-xl outline-none focus:border-primary text-sm"
                    />
                  </div>
                )}
                
                <div>
                  <label className="block text-xs font-bold text-text-muted mb-1 uppercase tracking-wider">Jumlah (Rp)</label>
                  <input 
                    type="number" 
                    required
                    min="1"
                    placeholder="Contoh: 500000"
                    value={modalForm.amount}
                    onChange={(e) => setModalForm({...modalForm, amount: e.target.value})}
                    className="w-full px-3 py-2 border border-border-main rounded-xl outline-none focus:border-primary text-sm"
                  />
                </div>

                <div>
                  <label className="block text-xs font-bold text-text-muted mb-1 uppercase tracking-wider">Tanggal Transaksi</label>
                  <input 
                    type="date" 
                    required
                    value={modalForm.transaction_date}
                    onChange={(e) => setModalForm({...modalForm, transaction_date: e.target.value})}
                    className="w-full px-3 py-2 border border-border-main rounded-xl outline-none focus:border-primary text-sm"
                  />
                </div>

                <div>
                  <label className="block text-xs font-bold text-text-muted mb-1 uppercase tracking-wider">Venue (Opsional)</label>
                  <select 
                    value={modalForm.venue_id}
                    onChange={(e) => setModalForm({...modalForm, venue_id: e.target.value})}
                    className="w-full px-3 py-2 border border-border-main rounded-xl outline-none focus:border-primary text-sm bg-white"
                  >
                    <option value="">-- Tidak Spesifik Venue --</option>
                    {venues.map(v => (
                      <option key={v.id} value={v.id}>{v.name}</option>
                    ))}
                  </select>
                </div>

                <div>
                  <label className="block text-xs font-bold text-text-muted mb-1 uppercase tracking-wider">Deskripsi (Opsional)</label>
                  <textarea 
                    rows={2}
                    placeholder="Keterangan tambahan..."
                    value={modalForm.description}
                    onChange={(e) => setModalForm({...modalForm, description: e.target.value})}
                    className="w-full px-3 py-2 border border-border-main rounded-xl outline-none focus:border-primary text-sm resize-none custom-scrollbar"
                  />
                </div>
              </form>
            </div>
            
            <div className="p-4 border-t border-border-main bg-gray-50 flex justify-end space-x-3 shrink-0">
              <button 
                type="button" 
                onClick={() => setIsModalOpen(false)}
                className="px-4 py-2 text-sm font-bold text-text-muted hover:text-text-main transition-colors"
                disabled={isSubmitting}
              >
                Batal
              </button>
              <button 
                form="createTxForm"
                type="submit"
                disabled={isSubmitting}
                className="px-6 py-2 text-sm font-bold bg-primary text-white rounded-xl hover:bg-primary/90 transition-colors disabled:opacity-50"
              >
                {isSubmitting ? 'Menyimpan...' : 'Simpan Transaksi'}
              </button>
            </div>
          </div>
        </div>
      )}
    </PageShell>
  );
};
