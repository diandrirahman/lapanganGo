export const isPortfolioDemo = import.meta.env.VITE_PORTFOLIO_DEMO === 'true';

export const PORTFOLIO_DEMO_STATE_KEY = 'lapanggo.portfolio-demo.state.v1';
export const PORTFOLIO_DEMO_SESSION_KEY = 'lapanggo.portfolio-demo.session.v1';

export type PortfolioDemoRole = 'CUSTOMER' | 'OWNER' | 'SUPER_ADMIN';
