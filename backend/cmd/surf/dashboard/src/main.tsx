import { render } from 'preact';
import { App } from './app';
import './styles/fonts.css';
import './styles/theme.css';
import './styles/app.css';

const root = document.getElementById('app');
if (root) render(<App />, root);
