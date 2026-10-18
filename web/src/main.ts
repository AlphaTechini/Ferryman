import { mount } from 'svelte';
import '@fontsource-variable/newsreader/opsz.css';
import '@fontsource-variable/geist';
import '@fontsource-variable/geist-mono';
import './styles/tokens.css';
import './styles/base.css';
import App from './App.svelte';

mount(App, { target: document.getElementById('app')! });
